package httpapi

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/auth"
	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/services"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/store/db"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
	"git.juzo.io/juzo/hashi/internal/vault"
)

// Review of #34: moving the admin domain onto a name another service uses must leave the settings unchanged, not save
// them while the system service stays behind (02 §1.6).
func TestAdminDomainChangeIsAllOrNothing(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	al := audit.New([]byte("k"))
	v, err := vault.New(bytes.Repeat([]byte{7}, vault.KeySize), 1)
	if err != nil {
		t.Fatal(err)
	}
	g := settings.General{RootDomain: "example.com", AdminDomain: "hashi.example.com", InternalURL: "http://192.0.2.5:3340",
		SyncIntervalSeconds: 3600, TrustedProxies: []string{}}
	if err := settings.Put(ctx, st.Q, g); err != nil {
		t.Fatal(err)
	}
	rec, _ := v.Seal(vault.Service, "dns.token", secrets.New("t"))
	_ = st.Q.InsertSecret(ctx, rec)
	_ = st.Q.InsertDNSProvider(ctx, db.InsertDNSProviderParams{ID: "p1", Name: "h", Type: "hetzner", DefaultTtl: 3600,
		TokenSecretID: rec.ID, Health: "healthy", CreatedAt: 1, UpdatedAt: 1})
	_ = st.Q.InsertDNSZone(ctx, db.InsertDNSZoneParams{ID: "z1", ProviderID: "p1", ProviderZoneID: "1", Name: "example.com", CreatedAt: 1})
	now := time.Now()
	if _, _, err := services.EnsureSystem(ctx, st.Q, now); err != nil {
		t.Fatal(err)
	}
	if _, err := services.Create(ctx, st.Q, services.Service{Name: "Search", Slug: "search", Kind: services.KindHTTP,
		DomainMode: services.ModeSubdomain, HostName: "search.example.com", Enabled: true,
		Target: services.Target{Address: "192.0.2.20", Port: 8080}}, now); err != nil {
		t.Fatal(err)
	}
	s := &Server{Store: st, Audit: al, Settings: settings.New(st), Sessions: auth.NewManager(st, al, []byte("csrf")), Log: slog.Default()}
	_, err = s.PutGeneralSettings(ctx, gen.PutGeneralSettingsRequestObject{Body: &gen.GeneralSettings{RootDomain: "example.com",
		AdminDomain: "search.example.com", InternalUrl: g.InternalURL, SyncIntervalSeconds: 3600, TrustedProxies: []string{}}})
	if err == nil {
		t.Fatal("moving the admin domain onto a used name succeeded")
	}
	got, _ := settings.Get[settings.General](ctx, st.R)
	sys, _ := st.R.GetServiceByOwner(ctx, services.OwnerSetup)
	if got.AdminDomain != "hashi.example.com" || sys.HostName != "hashi.example.com" {
		t.Errorf("after the failed change: admin domain %s, system service %s", got.AdminDomain, sys.HostName)
	}
}
