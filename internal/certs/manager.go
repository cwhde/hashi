package certs

import (
	"context"
	"crypto"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/dns"
	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/store"
	"git.juzo.io/juzo/hashi/internal/store/db"
	hsync "git.juzo.io/juzo/hashi/internal/sync"
	"git.juzo.io/juzo/hashi/internal/vault"
)

// Subsystem is the sync subsystem that issues and renews certificates. Its target is the ACME account ID (issue what
// is missing or due) or a certificate ID ("Renew now" for that certificate).
const Subsystem = "certificates"

// JobKind is the renewal job (hourly, JobInterval).
const JobKind = "certificates.renew"

// Vault purposes (service class: issuing runs unattended).
const (
	AccountKeyPurpose = "acme.account_key"
	EABPurpose        = "acme.eab_hmac"
	KeyPurpose        = "certificate.key"
)

// PropagationTimeout bounds the wait for a challenge value to show up on the checked DNS servers.
const PropagationTimeout = 5 * time.Minute

// ErrNotConfigured means setup step 4 has not stored a certificate provider yet.
var ErrNotConfigured = errors.New("no certificate provider is configured (setup step 4)")

// SolverSource opens the DNS-01 solver for the managed zone and returns the zone's authoritative nameservers.
type SolverSource func(ctx context.Context) (Solver, []string, error)

// DNSSolver is the production SolverSource: the managed zone's dns.Challenger.
func DNSSolver(st *store.Store, svc *dns.Service) SolverSource {
	return func(ctx context.Context) (Solver, []string, error) {
		zones, err := st.R.ListDNSZones(ctx)
		if err != nil {
			return nil, nil, err
		}
		if len(zones) == 0 {
			return nil, nil, errors.New("no DNS zone is configured (setup step 3)")
		}
		ch, err := svc.Challenger(ctx, zones[0].ID)
		if err != nil {
			return nil, nil, err
		}
		return ch, ch.Nameservers(), nil
	}
}

// Manager issues and renews Hashi's certificates through the sync engine (invariant 7).
type Manager struct {
	st     *store.Store
	vault  *vault.Vault
	audit  *audit.Log
	solver SolverSource
	http   *http.Client
	log    *slog.Logger
	now    func() time.Time
	// wait checks that a challenge value is visible; WaitTXT by default (tests shorten the poll).
	wait func(ctx context.Context, servers []string, fqdn, value string) error
	// newClient opens the ACME client: NewClient, or the dev build's local CA.
	newClient ClientFactory
}

// ACMEClient is what the manager needs from an ACME client (*Client).
type ACMEClient interface {
	Register(ctx context.Context) (string, error)
	Issue(ctx context.Context, solver Solver, names []string, opt IssueOptions) (Issued, error)
}

// ClientFactory opens an ACME client for an account.
type ClientFactory func(httpClient *http.Client, acct Account) ACMEClient

// UseClients replaces the ACME client (the dev build's local CA; never in production builds).
func (m *Manager) UseClients(f ClientFactory) { m.newClient = f }

// NewManager returns the certificate manager. httpClient talks to the CA (nil = http.DefaultClient).
func NewManager(st *store.Store, v *vault.Vault, a *audit.Log, solver SolverSource, httpClient *http.Client, log *slog.Logger) *Manager {
	return &Manager{st: st, vault: v, audit: a, solver: solver, http: httpClient, log: log.With("component", "certs"), now: time.Now,
		wait: func(ctx context.Context, servers []string, fqdn, value string) error {
			return WaitTXT(ctx, servers, fqdn, value, 5*time.Second)
		},
		newClient: func(h *http.Client, a Account) ACMEClient { return NewClient(h, a) }}
}

// Subsystems returns the sync subsystems to register with the engine.
func (m *Manager) Subsystems() []hsync.Subsystem { return []hsync.Subsystem{certSub{m}} }

type certSub struct{ m *Manager }

func (certSub) Name() string          { return Subsystem }
func (certSub) LockKey(string) string { return "certificates" }

// itemCert is a plan item's before/after: names and, before, what is installed now. No key material.
type itemCert struct {
	Name        string     `json:"name"`
	Names       []string   `json:"names"`
	NotAfter    *time.Time `json:"not_after,omitempty"`
	Fingerprint string     `json:"fingerprint,omitempty"`
}

func namesOf(r db.Certificate) []string {
	var n []string
	_ = json.Unmarshal([]byte(r.NamesJson), &n)
	return n
}

func (s certSub) Plan(ctx context.Context, target string) (hsync.Draft, error) {
	m := s.m
	acct, err := m.account(ctx)
	if err != nil {
		return hsync.Draft{}, err
	}
	general, err := settings.Get[settings.General](ctx, m.st.R)
	if err != nil {
		return hsync.Draft{}, err
	}
	want := Desired(general.RootDomain)
	if len(want) == 0 {
		return hsync.Draft{}, errors.New("the root domain is not set (setup step 2)")
	}
	rows, err := m.st.R.ListCertificates(ctx)
	if err != nil {
		return hsync.Draft{}, err
	}
	byName := map[string]db.Certificate{}
	forced := ""
	for _, r := range rows {
		byName[r.Name] = r
		if r.ID == target {
			forced = r.Name
		}
	}
	if target != acct.ID && forced == "" {
		return hsync.Draft{}, fmt.Errorf("no certificate or account %s", target)
	}
	now := m.now()
	var items []hsync.Item
	for _, w := range want {
		if forced != "" && w.Name != forced {
			continue
		}
		after, _ := json.Marshal(itemCert{Name: w.Name, Names: w.Names})
		item := hsync.Item{Action: hsync.ActionUpdate, Object: "certificate " + w.Name, After: after, Risk: hsync.Low}
		r, ok := byName[w.Name]
		if ok && forced == "" && r.LastRenewalResult == "failed" && r.LastRenewalAt.Valid &&
			now.Sub(time.UnixMilli(r.LastRenewalAt.Int64)) < RetryAfter {
			continue // the last attempt failed recently: wait before the next automatic one
		}
		if !ok || r.CertPem == "" {
			item.Action, item.Note = hsync.ActionCreate, "issue"
			items = append(items, item)
			continue
		}
		na := time.UnixMilli(r.NotAfter.Int64).UTC()
		item.Before, _ = json.Marshal(itemCert{Name: r.Name, Names: namesOf(r), NotAfter: &na, Fingerprint: r.Fingerprint})
		switch {
		case !SameNames(namesOf(r), w.Names):
			item.Note = "renew with the names now needed"
		case forced != "":
			item.Note = "renew now (requested)"
		case Due(now, na):
			item.Note = "renew: less than 30 days left"
		default:
			continue
		}
		items = append(items, item)
	}
	return hsync.Draft{Title: "Certificates for " + want[0].Name, Items: items}, nil
}

func (s certSub) Apply(ctx context.Context, target string, items []hsync.Item) (string, error) {
	m := s.m
	d, err := s.Plan(ctx, target)
	if err != nil {
		return "", err
	}
	if hsync.Hash(d.Items) != hsync.Hash(items) {
		return "", hsync.ErrReplanned
	}
	acct, err := m.account(ctx)
	if err != nil {
		return "", err
	}
	client, err := m.client(ctx, acct)
	if err != nil {
		return "", m.health(ctx, acct, err)
	}
	solver, ns, err := m.solver(ctx)
	if err != nil {
		return "", m.health(ctx, acct, err)
	}
	servers := ns
	var resolvers []string
	if json.Unmarshal([]byte(acct.ResolversJson), &resolvers) == nil && len(resolvers) > 0 {
		servers = resolvers
	}
	opt := IssueOptions{Delay: time.Duration(acct.PropagationDelayS) * time.Second,
		Wait: func(ctx context.Context, fqdn, value string) error {
			ctx, cancel := context.WithTimeout(ctx, PropagationTimeout)
			defer cancel()
			return m.wait(ctx, servers, fqdn, value)
		}}
	var done []string
	var errs []error
	for _, it := range items {
		var w itemCert
		if err := json.Unmarshal(it.After, &w); err != nil {
			return "", err
		}
		iss, err := client.Issue(ctx, solver, w.Names, opt)
		if iss.CertPEM == nil {
			errs = append(errs, fmt.Errorf("%s: %w", w.Name, err))
			m.failed(ctx, w.Name, err)
			continue
		}
		if serr := m.store(ctx, w, iss, it.Action == hsync.ActionCreate); serr != nil {
			return "", errors.Join(err, serr)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("%s issued, but: %w", w.Name, err))
		}
		done = append(done, fmt.Sprintf("%s (%d names, valid until %s)", w.Name, len(iss.Names), iss.NotAfter.UTC().Format(time.DateOnly)))
	}
	err = errors.Join(errs...)
	_ = m.health(ctx, acct, err)
	if len(done) == 0 {
		return "", err
	}
	return "issued " + strings.Join(done, ", "), err
}

// account returns the stored ACME account.
func (m *Manager) account(ctx context.Context) (db.AcmeAccount, error) {
	a, err := m.st.R.GetACMEAccount(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotConfigured
	}
	return a, err
}

// client opens the ACME client, creating the account key and registering the account on first use.
func (m *Manager) client(ctx context.Context, a db.AcmeAccount) (ACMEClient, error) {
	acct := Account{DirectoryURL: a.DirectoryUrl, Email: a.Email, EABKID: a.EabKid, URI: a.AccountUri}
	if a.EabHmacSecretID.Valid {
		v, err := m.secret(ctx, a.EabHmacSecretID.String)
		if err != nil {
			return nil, fmt.Errorf("EAB key: %w", err)
		}
		acct.EABHMAC = v
	}
	now := m.now().UnixMilli()
	if !a.AccountKeySecretID.Valid {
		_, pemKey, err := NewKey()
		if err != nil {
			return nil, err
		}
		rec, err := m.vault.Seal(vault.Service, AccountKeyPurpose, pemKey)
		if err != nil {
			return nil, err
		}
		if err := m.st.Tx(ctx, func(q *db.Queries) error {
			if err := q.InsertSecret(ctx, rec); err != nil {
				return err
			}
			a.AccountKeySecretID, a.AccountUri, a.UpdatedAt = sql.NullString{String: rec.ID, Valid: true}, "", now
			return q.UpsertACMEAccount(ctx, accountParams(a))
		}); err != nil {
			return nil, err
		}
		acct.URI = ""
	}
	v, err := m.secret(ctx, a.AccountKeySecretID.String)
	if err != nil {
		return nil, fmt.Errorf("ACME account key: %w", err)
	}
	key, err := ParseKey(v)
	if err != nil {
		return nil, fmt.Errorf("ACME account key: %w", err)
	}
	acct.Key = crypto.Signer(key)
	c := m.newClient(m.http, acct)
	if acct.URI == "" {
		uri, err := c.Register(ctx)
		if err != nil {
			return nil, err
		}
		a.AccountUri, a.UpdatedAt = uri, m.now().UnixMilli()
		if err := m.st.Q.UpsertACMEAccount(ctx, accountParams(a)); err != nil {
			return nil, err
		}
	}
	return c, nil
}

func accountParams(a db.AcmeAccount) db.UpsertACMEAccountParams {
	return db.UpsertACMEAccountParams(a)
}

// health records the outcome of talking to the CA on the account and returns err.
func (m *Manager) health(ctx context.Context, a db.AcmeAccount, err error) error {
	cur, gerr := m.st.R.GetACMEAccount(ctx) // re-read: client may have stored a key or URI
	if gerr != nil {
		return err
	}
	a = cur
	now := m.now().UnixMilli()
	a.Health, a.LastValidationError = "healthy", ""
	if err != nil {
		a.Health, a.LastValidationError = "failed", err.Error()
	}
	a.LastValidatedAt, a.UpdatedAt = sql.NullInt64{Int64: now, Valid: true}, now
	_ = m.st.Q.UpsertACMEAccount(context.WithoutCancel(ctx), accountParams(a))
	return err
}

func (m *Manager) secret(ctx context.Context, id string) (secrets.Value, error) {
	rec, err := m.st.R.GetSecret(ctx, id)
	if err != nil {
		return secrets.Value{}, err
	}
	return m.vault.Open(rec)
}

// store seals the new key and records the certificate; the previous key secret is deleted with it.
func (m *Manager) store(ctx context.Context, w itemCert, iss Issued, first bool) error {
	rec, err := m.vault.Seal(vault.Service, KeyPurpose, iss.KeyPEM)
	if err != nil {
		return err
	}
	names, _ := json.Marshal(w.Names)
	now := m.now().UnixMilli()
	return m.st.Tx(ctx, func(q *db.Queries) error {
		if err := q.InsertSecret(ctx, rec); err != nil {
			return err
		}
		rows, err := q.ListCertificates(ctx)
		if err != nil {
			return err
		}
		var cur *db.Certificate
		for i := range rows {
			if rows[i].Name == w.Name {
				cur = &rows[i]
			}
		}
		if cur == nil {
			id := ulid.Make().String()
			if err := q.InsertCertificate(ctx, db.InsertCertificateParams{ID: id, Name: w.Name, NamesJson: string(names),
				CreatedAt: now, UpdatedAt: now}); err != nil {
				return err
			}
			cur = &db.Certificate{ID: id}
		}
		result := "renewed"
		if first || cur.CertPem == "" {
			result = "issued"
		}
		if err := q.SetCertificateIssued(ctx, db.SetCertificateIssuedParams{NamesJson: string(names),
			KeySecretID: sql.NullString{String: rec.ID, Valid: true}, CertPem: string(iss.CertPEM), Issuer: iss.Issuer,
			NotBefore: sql.NullInt64{Int64: iss.NotBefore.UnixMilli(), Valid: true}, NotAfter: sql.NullInt64{Int64: iss.NotAfter.UnixMilli(), Valid: true},
			Fingerprint: iss.Fingerprint, LastRenewalAt: sql.NullInt64{Int64: now, Valid: true}, LastRenewalResult: result,
			UpdatedAt: now, ID: cur.ID}); err != nil {
			return err
		}
		if cur.KeySecretID.Valid {
			return q.DeleteSecret(ctx, cur.KeySecretID.String)
		}
		return nil
	})
}

// failed records a failed issue or renewal on the certificate row (created if this was its first attempt).
func (m *Manager) failed(ctx context.Context, name string, cause error) {
	ctx = context.WithoutCancel(ctx)
	now := m.now().UnixMilli()
	err := m.st.Tx(ctx, func(q *db.Queries) error {
		rows, err := q.ListCertificates(ctx)
		if err != nil {
			return err
		}
		for _, r := range rows {
			if r.Name == name {
				return q.SetCertificateRenewalFailed(ctx, db.SetCertificateRenewalFailedParams{
					LastRenewalAt: sql.NullInt64{Int64: now, Valid: true}, LastRenewalError: cause.Error(), UpdatedAt: now, ID: r.ID})
			}
		}
		id := ulid.Make().String()
		names, _ := json.Marshal(Desired(name)[0].Names)
		if err := q.InsertCertificate(ctx, db.InsertCertificateParams{ID: id, Name: name, NamesJson: string(names), CreatedAt: now, UpdatedAt: now}); err != nil {
			return err
		}
		return q.SetCertificateRenewalFailed(ctx, db.SetCertificateRenewalFailedParams{
			LastRenewalAt: sql.NullInt64{Int64: now, Valid: true}, LastRenewalError: cause.Error(), UpdatedAt: now, ID: id})
	})
	if err != nil {
		m.log.Error("record certificate failure", "certificate", name, "err", err)
	}
}

// Distributor plans certificate distribution to every Traefik host whose files lag behind (the traefik subsystem).
type Distributor func(ctx context.Context) (string, error)

// Renew is the certificates.renew job (daily): issue or renew through an automatic plan, distribute, and report
// certificates whose expiry is getting close (warning at 21 days, critical at 7).
func (m *Manager) Renew(eng *hsync.Engine, distribute Distributor) func(ctx context.Context) (string, error) {
	return func(ctx context.Context) (string, error) {
		acct, err := m.account(ctx)
		if errors.Is(err, ErrNotConfigured) {
			return "no certificate provider configured", nil
		}
		if err != nil {
			return "", err
		}
		var parts []string
		var errs []error
		p, err := eng.Auto(ctx, Subsystem, acct.ID, hsync.TriggerReconcile, audit.System)
		switch {
		case err != nil:
			errs = append(errs, err)
		case p == nil:
			parts = append(parts, "certificates current")
		default:
			parts = append(parts, fmt.Sprintf("certificate plan %s", p.Status))
		}
		if distribute != nil {
			sum, err := distribute(ctx)
			if err != nil {
				errs = append(errs, err)
			}
			if sum != "" {
				parts = append(parts, sum)
			}
		}
		rows, err := m.st.R.ListCertificates(ctx)
		if err != nil {
			return "", err
		}
		for _, r := range rows {
			if !r.NotAfter.Valid {
				continue
			}
			na := time.UnixMilli(r.NotAfter.Int64)
			if lvl := LevelOf(m.now(), na); lvl != LevelOK {
				days := int(na.Sub(m.now()).Hours() / 24)
				m.log.Warn("certificate expiry", "certificate", r.Name, "level", string(lvl), "days_left", days)
				parts = append(parts, fmt.Sprintf("%s %s: %d days left", r.Name, lvl, days))
			}
		}
		return strings.Join(parts, "; "), errors.Join(errs...)
	}
}
