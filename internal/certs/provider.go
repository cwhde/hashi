package certs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/url"
	"strings"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/store/db"
	"git.juzo.io/juzo/hashi/internal/vault"
)

// ErrInvalid means the certificate provider input is not acceptable; the message says what to change.
var ErrInvalid = errors.New("invalid certificate provider")

// ProviderInput is setup step 4 (02 §1.4). An empty EABHMAC keeps the stored key.
type ProviderInput struct {
	Directory         string // gts, letsencrypt or custom
	DirectoryURL      string // custom only
	Email             string
	EABKID            string
	EABHMAC           secrets.Value
	PropagationDelayS int
	Resolvers         []string // host or host:port; empty = the zone's authoritative nameservers
}

// AccountID is the ID of Hashi's one ACME account row.
const AccountID = "acme"

// Configure stores the certificate provider and registers the ACME account with the CA (02 §1.4: the step completes
// only once the account is registered). A changed CA, EAB key ID or email starts a new account with a new key.
func (m *Manager) Configure(ctx context.Context, actor, ip string, in ProviderInput) (db.AcmeAccount, error) {
	in.Email, in.EABKID = strings.TrimSpace(in.Email), strings.TrimSpace(in.EABKID)
	dirURL, ok := Directories[in.Directory]
	switch {
	case in.Directory == "custom":
		u, err := url.Parse(strings.TrimSpace(in.DirectoryURL))
		if err != nil || u.Scheme != "https" || u.Host == "" {
			return db.AcmeAccount{}, fmt.Errorf("%w: a custom ACME directory must be an https URL", ErrInvalid)
		}
		dirURL = u.String()
	case !ok:
		return db.AcmeAccount{}, fmt.Errorf("%w: unknown ACME directory %q", ErrInvalid, in.Directory)
	}
	if a, err := mail.ParseAddress(in.Email); err != nil || a.Address != in.Email {
		return db.AcmeAccount{}, fmt.Errorf("%w: %q is not an email address", ErrInvalid, in.Email)
	}
	if in.PropagationDelayS < 0 || in.PropagationDelayS > 600 {
		return db.AcmeAccount{}, fmt.Errorf("%w: the propagation delay must be between 0 and 600 seconds", ErrInvalid)
	}
	for _, r := range in.Resolvers {
		host := r
		if h, _, err := net.SplitHostPort(r); err == nil {
			host = h
		}
		if host == "" || strings.ContainsAny(host, " /") {
			return db.AcmeAccount{}, fmt.Errorf("%w: %q is not a DNS server address", ErrInvalid, r)
		}
	}
	provs, err := m.st.R.ListDNSProviders(ctx)
	if err != nil {
		return db.AcmeAccount{}, err
	}
	if len(provs) == 0 {
		return db.AcmeAccount{}, fmt.Errorf("%w: connect the DNS provider first (step 3)", ErrInvalid)
	}
	cur, err := m.st.R.GetACMEAccount(ctx)
	exists := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return cur, err
	}
	keepHMAC := exists && in.EABHMAC.IsZero() && cur.EabKid == in.EABKID && cur.EabHmacSecretID.Valid
	if in.EABKID != "" && in.EABHMAC.IsZero() && !keepHMAC {
		return cur, fmt.Errorf("%w: the EAB HMAC key is required with a key ID", ErrInvalid)
	}
	if !in.EABHMAC.IsZero() {
		if _, err := decodeHMAC(in.EABHMAC.Reveal()); err != nil {
			return cur, fmt.Errorf("%w: %w", ErrInvalid, err)
		}
	}
	if in.Directory == "gts" && in.EABKID == "" {
		return cur, fmt.Errorf("%w: Google Trust Services needs an EAB key ID and HMAC key", ErrInvalid)
	}
	resolvers, _ := json.Marshal(append([]string{}, in.Resolvers...))
	now := m.now().UnixMilli()
	a := db.AcmeAccount{ID: AccountID, Directory: in.Directory, DirectoryUrl: dirURL, Email: in.Email, EabKid: in.EABKID,
		DnsProviderID: provs[0].ID, PropagationDelayS: int64(in.PropagationDelayS), ResolversJson: string(resolvers),
		Health: "validating", CreatedAt: now, UpdatedAt: now}
	var stale []string // secrets replaced by this save
	if exists {
		a.ID, a.CreatedAt = cur.ID, cur.CreatedAt
		if cur.DirectoryUrl == dirURL && cur.EabKid == in.EABKID && cur.Email == in.Email {
			a.AccountKeySecretID, a.AccountUri = cur.AccountKeySecretID, cur.AccountUri
		} else if cur.AccountKeySecretID.Valid {
			stale = append(stale, cur.AccountKeySecretID.String)
		}
		if keepHMAC {
			a.EabHmacSecretID = cur.EabHmacSecretID
		} else if cur.EabHmacSecretID.Valid {
			stale = append(stale, cur.EabHmacSecretID.String)
		}
	}
	var hmacRec *db.InsertSecretParams
	if !in.EABHMAC.IsZero() {
		rec, err := m.vault.Seal(vault.Service, EABPurpose, in.EABHMAC)
		if err != nil {
			return cur, err
		}
		hmacRec, a.EabHmacSecretID = &rec, sql.NullString{String: rec.ID, Valid: true}
	}
	if err := m.st.Tx(ctx, func(q *db.Queries) error {
		if hmacRec != nil {
			if err := q.InsertSecret(ctx, *hmacRec); err != nil {
				return err
			}
		}
		if err := q.UpsertACMEAccount(ctx, accountParams(a)); err != nil {
			return err
		}
		for _, id := range stale {
			if err := q.DeleteSecret(ctx, id); err != nil {
				return err
			}
		}
		return m.audit.Record(ctx, q, audit.Event{Actor: actor, Action: "certificates.provider.saved", IP: ip, Target: "acme_account:" + a.ID,
			Detail: map[string]any{"directory": dirURL, "email": in.Email, "eab_kid": in.EABKID}})
	}); err != nil {
		return a, err
	}
	_, rerr := m.client(ctx, a)
	_ = m.health(ctx, a, rerr)
	out, err := m.st.R.GetACMEAccount(ctx)
	if err != nil {
		return out, err
	}
	return out, rerr
}
