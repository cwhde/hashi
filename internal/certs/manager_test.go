package certs

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/dns"
	"git.juzo.io/juzo/hashi/internal/dns/hetzner"
	"git.juzo.io/juzo/hashi/internal/dns/hetzner/hetznertest"
	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/services"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/store"
	"git.juzo.io/juzo/hashi/internal/store/db"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
	hsync "git.juzo.io/juzo/hashi/internal/sync"
	"git.juzo.io/juzo/hashi/internal/vault"
)

// fixture is a store with the root domain set and a connected DNS provider (the simulated Hetzner zone example.com),
// the state setup steps 2 and 3 leave behind.
type fixture struct {
	st    *store.Store
	vault *vault.Vault
	dns   *dns.Service
	sim   *hetznertest.Sim
	prov  string
	audit *audit.Log
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	ctx := context.Background()
	st := storetest.New(t)
	v, err := vault.New(bytes.Repeat([]byte{7}, vault.KeySize), 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := settings.Put(ctx, st.Q, settings.General{RootDomain: "example.com", AdminDomain: "hashi.example.com",
		InternalURL: "http://192.0.2.5:3340", SyncIntervalSeconds: 3600}); err != nil {
		t.Fatal(err)
	}
	sim, srv := hetznertest.NewSimServer(t)
	a := audit.New([]byte("k"))
	svc := dns.NewService(st, v, a, func(_ string, token secrets.Value) (dns.Provider, error) {
		return hetzner.New(token, hetzner.WithBaseURL(srv.URL+"/v1"),
			hetzner.WithClock(time.Now, func(context.Context, time.Duration) error { return nil })), nil
	})
	res, err := svc.Connect(ctx, "test", "", dns.ConnectInput{Type: "hetzner", Token: secrets.New(hetznertest.Token),
		DefaultTTL: 3600, RootDomain: "example.com"})
	if err != nil {
		t.Fatal(err)
	}
	return fixture{st: st, vault: v, dns: svc, sim: sim, prov: res.Provider.ID, audit: a}
}

// account stores the ACME account setup step 4 would (EAB secret sealed when hmac is set).
func (f fixture) account(t *testing.T, dirURL, kid, hmac string, resolvers string) db.AcmeAccount {
	t.Helper()
	ctx := context.Background()
	a := db.AcmeAccount{ID: "acme1", Directory: "custom", DirectoryUrl: dirURL, Email: "admin@example.com", EabKid: kid,
		DnsProviderID: f.prov, ResolversJson: resolvers, Health: "unknown", CreatedAt: 1, UpdatedAt: 1}
	if hmac != "" {
		rec, err := f.vault.Seal(vault.Service, EABPurpose, secrets.New(hmac))
		if err != nil {
			t.Fatal(err)
		}
		if err := f.st.Q.InsertSecret(ctx, rec); err != nil {
			t.Fatal(err)
		}
		a.EabHmacSecretID = sql.NullString{String: rec.ID, Valid: true}
	}
	if err := f.st.Q.UpsertACMEAccount(ctx, accountParams(a)); err != nil {
		t.Fatal(err)
	}
	return a
}

func itemsNotes(items []hsync.Item) string {
	var n []string
	for _, it := range items {
		n = append(n, it.Action+" "+it.Object+": "+it.Note)
	}
	return strings.Join(n, "; ")
}

// The certificates plan: nothing without a provider; issue when missing; nothing while valid; renew inside 30 days or
// when the names change; "Renew now" targets one certificate. No item carries key material.
func TestPlanIssuesRenewsAndForces(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	m := NewManager(f.st, f.vault, f.audit, nil, nil, slog.Default())
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	m.now = func() time.Time { return now }
	sub := certSub{m}
	if _, err := sub.Plan(ctx, "acme1"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("no provider: %v", err)
	}
	f.account(t, "https://ca.example.test/dir", "", "", "[]")

	d, err := sub.Plan(ctx, "acme1")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Items) != 1 || d.Items[0].Action != hsync.ActionCreate || hsync.RiskOf(d.Items) != hsync.Low ||
		!strings.Contains(string(d.Items[0].After), `"*.example.com"`) {
		t.Fatalf("missing certificate: %s %s", itemsNotes(d.Items), d.Items[0].After)
	}

	key := secrets.New("k")
	rec, _ := f.vault.Seal(vault.Service, KeyPurpose, key)
	_ = f.st.Q.InsertSecret(ctx, rec)
	_ = f.st.Q.InsertCertificate(ctx, db.InsertCertificateParams{ID: "cert1", Name: "example.com", NamesJson: `["example.com","*.example.com"]`})
	issued := func(notAfter time.Time, names string) {
		if err := f.st.Q.SetCertificateIssued(ctx, db.SetCertificateIssuedParams{NamesJson: names,
			KeySecretID: sql.NullString{String: rec.ID, Valid: true}, CertPem: "PEM", Fingerprint: "ab",
			NotAfter: sql.NullInt64{Int64: notAfter.UnixMilli(), Valid: true}, LastRenewalResult: "issued", ID: "cert1"}); err != nil {
			t.Fatal(err)
		}
	}
	issued(now.Add(31*24*time.Hour), `["*.example.com","example.com"]`)
	if d, _ := sub.Plan(ctx, "acme1"); len(d.Items) != 0 {
		t.Errorf("valid certificate: %s", itemsNotes(d.Items))
	}
	if d, _ := sub.Plan(ctx, "cert1"); len(d.Items) != 1 || d.Items[0].Note != "renew now (requested)" {
		t.Errorf("forced: %s", itemsNotes(d.Items))
	}
	issued(now.Add(30*24*time.Hour), `["example.com","*.example.com"]`)
	if d, _ := sub.Plan(ctx, "acme1"); len(d.Items) != 1 || d.Items[0].Action != hsync.ActionUpdate || !strings.Contains(d.Items[0].Note, "30 days") {
		t.Errorf("due: %s", itemsNotes(d.Items))
	}
	issued(now.Add(80*24*time.Hour), `["example.com"]`)
	if d, _ := sub.Plan(ctx, "acme1"); len(d.Items) != 1 || !strings.Contains(d.Items[0].Note, "names") {
		t.Errorf("names changed: %s", itemsNotes(d.Items))
	}
	for _, it := range d.Items {
		if bytes.Contains(it.Before, []byte("PRIVATE")) || bytes.Contains(it.After, []byte("PRIVATE")) {
			t.Error("key material in a plan item")
		}
	}
	// A failed attempt waits RetryAfter before the job tries again; "Renew now" does not wait.
	issued(now.Add(10*24*time.Hour), `["example.com","*.example.com"]`)
	_ = f.st.Q.SetCertificateRenewalFailed(ctx, db.SetCertificateRenewalFailedParams{
		LastRenewalAt: sql.NullInt64{Int64: now.Add(-time.Hour).UnixMilli(), Valid: true}, LastRenewalError: "CA down", ID: "cert1"})
	if d, _ := sub.Plan(ctx, "acme1"); len(d.Items) != 0 {
		t.Errorf("retry within the backoff: %s", itemsNotes(d.Items))
	}
	if d, _ := sub.Plan(ctx, "cert1"); len(d.Items) != 1 {
		t.Errorf("renew now during the backoff: %s", itemsNotes(d.Items))
	}
	now = now.Add(RetryAfter)
	if d, _ := sub.Plan(ctx, "acme1"); len(d.Items) != 1 {
		t.Errorf("retry after the backoff: %s", itemsNotes(d.Items))
	}
	if _, err := sub.Plan(ctx, "nope"); err == nil {
		t.Error("unknown target accepted")
	}
}

// Review of #33: after a failed first issue, the job waits RetryAfter, but the setup click ("Issue now", "Try again")
// targets the certificate and tries at once.
func TestIssueNowRetriesAfterFailedFirstIssue(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	m := NewManager(f.st, f.vault, f.audit, nil, nil, slog.Default())
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	m.now = func() time.Time { return now }
	f.account(t, "https://ca.example.test/dir", "", "", "[]")
	m.failed(ctx, "example.com", []string{"example.com", "*.example.com"}, errors.New("the CA did not validate example.com"))
	sub := certSub{m}
	if d, _ := sub.Plan(ctx, "acme1"); len(d.Items) != 0 {
		t.Fatalf("the job retries within the wait: %s", itemsNotes(d.Items))
	}
	target, err := m.IssueTarget(ctx)
	if err != nil || target == "acme1" {
		t.Fatalf("target %q %v", target, err)
	}
	if d, _ := sub.Plan(ctx, target); len(d.Items) != 1 || d.Items[0].Action != hsync.ActionCreate {
		t.Errorf("issue now after a failure: %s", itemsNotes(d.Items))
	}
}

// 03 §5.3: a service on a deeper name makes the certificate gain that parent's wildcard; a wildcard no longer needed
// stays until the next renewal instead of forcing one.
func TestPlanAddsDeeperWildcardForServices(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	m := NewManager(f.st, f.vault, f.audit, nil, nil, slog.Default())
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	m.now = func() time.Time { return now }
	sub := certSub{m}
	f.account(t, "https://ca.example.test/dir", "", "", "[]")
	rec, _ := f.vault.Seal(vault.Service, KeyPurpose, secrets.New("k"))
	_ = f.st.Q.InsertSecret(ctx, rec)
	_ = f.st.Q.InsertCertificate(ctx, db.InsertCertificateParams{ID: "cert1", Name: "example.com", NamesJson: `[]`})
	issued := func(names string) {
		if err := f.st.Q.SetCertificateIssued(ctx, db.SetCertificateIssuedParams{NamesJson: names,
			KeySecretID: sql.NullString{String: rec.ID, Valid: true}, CertPem: "PEM", Fingerprint: "ab",
			NotAfter: sql.NullInt64{Int64: now.Add(80 * 24 * time.Hour).UnixMilli(), Valid: true}, LastRenewalResult: "issued", ID: "cert1"}); err != nil {
			t.Fatal(err)
		}
	}
	issued(`["example.com","*.example.com"]`)
	svc, err := services.Create(ctx, f.st.Q, services.Service{Name: "Hashi dev", Slug: "dev", Kind: services.KindHTTP,
		DomainMode: services.ModeSubdomain, HostName: "dev.hashi.example.com", Enabled: true,
		Target: services.Target{Address: "192.0.2.5", Port: 3340}}, now)
	if err != nil {
		t.Fatal(err)
	}
	d, _ := sub.Plan(ctx, "acme1")
	if len(d.Items) != 1 || !strings.Contains(d.Items[0].Note, "names") || !strings.Contains(string(d.Items[0].After), `"*.hashi.example.com"`) {
		t.Fatalf("deeper name: %s", itemsNotes(d.Items))
	}
	issued(`["example.com","*.example.com","*.hashi.example.com","*.old.example.com"]`)
	if d, _ := sub.Plan(ctx, "acme1"); len(d.Items) != 0 {
		t.Errorf("covered, with a stale wildcard: %s", itemsNotes(d.Items))
	}
	if _, err := services.SetEnabled(ctx, f.st.Q, svc.ID, false, now); err != nil {
		t.Fatal(err)
	}
	if d, _ := sub.Plan(ctx, "acme1"); len(d.Items) != 0 {
		t.Errorf("service disabled mid-term: %s", itemsNotes(d.Items))
	}
	// "Renew now" issues exactly the names needed now: the stale wildcards go.
	d, _ = sub.Plan(ctx, "cert1")
	if len(d.Items) != 1 || strings.Contains(string(d.Items[0].After), "old") || strings.Contains(string(d.Items[0].After), "hashi") {
		t.Errorf("renew now: %s %s", itemsNotes(d.Items), d.Items[0].After)
	}
}
