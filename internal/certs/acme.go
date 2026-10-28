package certs

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/acme"

	"git.juzo.io/juzo/hashi/internal/secrets"
)

// Solver publishes and removes DNS-01 values (dns.Challenger in production).
type Solver interface {
	Present(ctx context.Context, fqdn, value string) error
	CleanUp(ctx context.Context, fqdn, value string) error
}

// Account is Hashi's ACME account.
type Account struct {
	DirectoryURL string
	Email        string
	// EABKID and EABHMAC are the External Account Binding (GTS requires it); the HMAC is the base64url key the CA shows.
	EABKID  string
	EABHMAC secrets.Value
	Key     crypto.Signer
	// URI is the account URL once registered ("" = not yet).
	URI string
}

// Client talks ACME (RFC 8555) to one CA with one account.
type Client struct {
	c    *acme.Client
	acct Account
}

// NewClient returns a client for acct. httpClient nil means http.DefaultClient.
func NewClient(httpClient *http.Client, acct Account) *Client {
	c := &acme.Client{Key: acct.Key, DirectoryURL: acct.DirectoryURL, HTTPClient: httpClient, UserAgent: "hashi"}
	if acct.URI != "" {
		c.KID = acme.KeyID(acct.URI)
	}
	return &Client{c: c, acct: acct}
}

// Register creates the account (with EAB when a key ID is set) or finds the existing one for this key, and returns
// its URL.
func (c *Client) Register(ctx context.Context) (string, error) {
	a := &acme.Account{}
	if c.acct.Email != "" {
		a.Contact = []string{"mailto:" + c.acct.Email}
	}
	if c.acct.EABKID != "" {
		mac, err := decodeHMAC(c.acct.EABHMAC.Reveal())
		if err != nil {
			return "", err
		}
		a.ExternalAccountBinding = &acme.ExternalAccountBinding{KID: c.acct.EABKID, Key: mac}
	}
	got, err := c.c.Register(ctx, a, acme.AcceptTOS)
	if errors.Is(err, acme.ErrAccountAlreadyExists) {
		got, err = c.c.GetReg(ctx, "")
	}
	if err != nil {
		return "", fmt.Errorf("register the ACME account: %w", err)
	}
	c.acct.URI = got.URI
	return got.URI, nil
}

// decodeHMAC accepts the EAB key as CAs show it: base64url, with or without padding.
func decodeHMAC(s string) ([]byte, error) {
	s = strings.TrimRight(strings.TrimSpace(s), "=")
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil || len(b) == 0 {
		return nil, errors.New("the EAB HMAC key is not base64url")
	}
	return b, nil
}

// IssueOptions control the DNS-01 wait.
type IssueOptions struct {
	// Wait returns once value is visible at fqdn (WaitTXT against the zone's nameservers or the configured resolvers).
	Wait func(ctx context.Context, fqdn, value string) error
	// Delay is the extra pause after Wait before the CA is asked to check (setting "propagation delay").
	Delay time.Duration
}

type challenge struct {
	authz      string
	fqdn, val  string
	chal       *acme.Challenge
	identifier string
}

// Issue orders a certificate for names, solves every DNS-01 challenge through solver, and returns the certificate
// with a fresh key. Challenge values are removed afterwards whatever happened; a failed removal is reported in the
// error only when issuing itself succeeded (the certificate is returned with it).
func (c *Client) Issue(ctx context.Context, solver Solver, names []string, opt IssueOptions) (iss Issued, err error) {
	order, err := c.c.AuthorizeOrder(ctx, acme.DomainIDs(names...))
	if err != nil {
		return Issued{}, fmt.Errorf("create the order: %w", err)
	}
	var todo []challenge
	defer func() {
		var errs []error
		cctx := context.WithoutCancel(ctx)
		for _, t := range todo {
			if e := solver.CleanUp(cctx, t.fqdn, t.val); e != nil {
				errs = append(errs, fmt.Errorf("remove the challenge value at %s: %w", t.fqdn, e))
			}
		}
		if err == nil && len(errs) > 0 {
			err = errors.Join(errs...)
		}
	}()
	for _, u := range order.AuthzURLs {
		z, err := c.c.GetAuthorization(ctx, u)
		if err != nil {
			return Issued{}, fmt.Errorf("read the authorization: %w", err)
		}
		if z.Status == acme.StatusValid {
			continue // the CA still remembers this name as validated
		}
		var ch *acme.Challenge
		for _, x := range z.Challenges {
			if x.Type == "dns-01" {
				ch = x
			}
		}
		if ch == nil {
			return Issued{}, fmt.Errorf("the CA offers no dns-01 challenge for %s", z.Identifier.Value)
		}
		val, err := c.c.DNS01ChallengeRecord(ch.Token)
		if err != nil {
			return Issued{}, err
		}
		t := challenge{authz: u, fqdn: "_acme-challenge." + strings.TrimPrefix(z.Identifier.Value, "*."), val: val, chal: ch,
			identifier: z.Identifier.Value}
		if err := solver.Present(ctx, t.fqdn, t.val); err != nil {
			return Issued{}, fmt.Errorf("publish the challenge for %s: %w", t.identifier, err)
		}
		todo = append(todo, t)
	}
	for _, t := range todo {
		if opt.Wait != nil {
			if err := opt.Wait(ctx, t.fqdn, t.val); err != nil {
				return Issued{}, err
			}
		}
	}
	if len(todo) > 0 && opt.Delay > 0 {
		select {
		case <-ctx.Done():
			return Issued{}, ctx.Err()
		case <-time.After(opt.Delay):
		}
	}
	for _, t := range todo {
		if _, err := c.c.Accept(ctx, t.chal); err != nil {
			return Issued{}, fmt.Errorf("ask the CA to check %s: %w", t.identifier, err)
		}
	}
	for _, t := range todo {
		if _, err := c.c.WaitAuthorization(ctx, t.authz); err != nil {
			return Issued{}, fmt.Errorf("the CA did not validate %s: %w", t.identifier, err)
		}
	}
	if order, err = c.c.WaitOrder(ctx, order.URI); err != nil {
		return Issued{}, fmt.Errorf("the order did not become ready: %w", err)
	}
	key, keyPEM, err := NewKey()
	if err != nil {
		return Issued{}, err
	}
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: names[0]},
		DNSNames: names}, key)
	if err != nil {
		return Issued{}, err
	}
	// CreateCertFromOrder keeps the order URL, needed when the CA answers finalize with "processing" (Pebble does).
	chain, _, err := c.c.CreateCertFromOrder(ctx, order, csr, true)
	if err != nil {
		return Issued{}, fmt.Errorf("finalize the order: %w", err)
	}
	return FromChain(chain, keyPEM)
}
