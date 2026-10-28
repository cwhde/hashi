// Package certstest is a local certificate authority for `make dev` and the E2E suite (dev builds only). It stands in
// for the ACME server: registration always succeeds, and issuing still publishes and removes one DNS-01 value per name
// through the real solver (the DNS adapter), then signs the certificate itself.
package certstest

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"

	"git.juzo.io/juzo/hashi/internal/certs"
)

// CA signs certificates with a throwaway root.
type CA struct {
	key  *ecdsa.PrivateKey
	root *x509.Certificate
	der  []byte
}

// NewCA creates the root.
func NewCA() (*CA, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	tpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{Organization: []string{"Hashi dev"}, CommonName: "Hashi dev CA"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(10 * 365 * 24 * time.Hour), IsCA: true,
		BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	root, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	return &CA{key: key, root: root, der: der}, nil
}

// Clients is a certs.ClientFactory.
func (ca *CA) Clients(_ *http.Client, a certs.Account) certs.ACMEClient {
	return client{ca: ca, acct: a}
}

type client struct {
	ca   *CA
	acct certs.Account
}

func (c client) Register(context.Context) (string, error) {
	if c.acct.EABKID != "" && c.acct.EABHMAC.IsZero() {
		return "", fmt.Errorf("register the ACME account: the CA requires the EAB HMAC key")
	}
	return "https://ca.dev.invalid/acct/1", nil
}

func (c client) Issue(ctx context.Context, solver certs.Solver, names []string, _ certs.IssueOptions) (certs.Issued, error) {
	var presented [][2]string
	defer func() {
		for _, p := range presented {
			_ = solver.CleanUp(context.WithoutCancel(ctx), p[0], p[1])
		}
	}()
	for _, n := range names {
		b := make([]byte, 16)
		_, _ = rand.Read(b)
		fqdn, val := "_acme-challenge."+strings.TrimPrefix(n, "*."), hex.EncodeToString(b)
		if err := solver.Present(ctx, fqdn, val); err != nil {
			return certs.Issued{}, fmt.Errorf("publish the challenge for %s: %w", n, err)
		}
		presented = append(presented, [2]string{fqdn, val})
	}
	key, keyPEM, err := certs.NewKey()
	if err != nil {
		return certs.Issued{}, err
	}
	serial, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
	tpl := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: names[0]}, DNSNames: names,
		NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(90 * 24 * time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, tpl, c.ca.root, &key.PublicKey, c.ca.key)
	if err != nil {
		return certs.Issued{}, err
	}
	return certs.FromChain([][]byte{der, c.ca.der}, keyPEM)
}
