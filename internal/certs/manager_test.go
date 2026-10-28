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
	if _, err := sub.Plan(ctx, "nope"); err == nil {
		t.Error("unknown target accepted")
	}
}
