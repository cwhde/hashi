package firewall

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"git.juzo.io/juzo/hashi/internal/ipsets"
	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/services"
	"git.juzo.io/juzo/hashi/internal/store/db"
	"git.juzo.io/juzo/hashi/internal/vault"
)

// Bug 20: a service says where it is published, and why nothing is when no firewall host has its upstream.
func TestPublication(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	v, _ := vault.New(bytes.Repeat([]byte{7}, vault.KeySize), 1)
	rec, _ := v.Seal(vault.Service, "dns.token", secrets.New("t"))
	if err := e.st.Q.InsertSecret(ctx, rec); err != nil {
		t.Fatal(err)
	}
	if err := e.st.Q.InsertDNSProvider(ctx, db.InsertDNSProviderParams{ID: "p1", Name: "hetzner", Type: "hetzner", DefaultTtl: 3600,
		TokenSecretID: rec.ID, Health: "healthy", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := e.st.Q.InsertDNSZone(ctx, db.InsertDNSZoneParams{ID: "z1", ProviderID: "p1", ProviderZoneID: "1", Name: "example.com", ZoneTtl: 3600, CreatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	in := managedInput(t)
	in.PublicIPOverride = "198.51.100.1"
	if _, err := e.m.Save(ctx, "admin", "", "", in); err != nil {
		t.Fatal(err)
	}
	mk := func(slug, addr string) string {
		s, err := services.Create(ctx, e.st.Q, services.Service{Name: slug, Slug: slug, Kind: services.KindHTTP, DomainMode: services.ModeSubdomain,
			HostName: slug + ".example.com", Enabled: true, AdGuardRewrite: true, Target: services.Target{Address: addr, Port: 80}}, now)
		if err != nil {
			t.Fatal(err)
		}
		return s.ID
	}
	resolve := func(context.Context, string, string) (ipsets.Target, error) { return ipsets.Target{}, nil }
	p, err := e.m.Publication(ctx, resolve, mk("wiki", "10.32.0.20"))
	if err != nil || p.HostName != "edge-a" || p.Reason != "" || p.Rewrite != "10.32.0.5" || len(p.Records) != 1 ||
		p.Records[0].Values[0] != "on.edge-a.example.com." {
		t.Fatalf("behind edge-a: %+v %v", p, err)
	}
	p, err = e.m.Publication(ctx, resolve, mk("nas", "10.99.0.5"))
	if err != nil || p.HostID != "" || len(p.Records) != 0 || !strings.Contains(p.Reason, "doesn't know which entry") {
		t.Fatalf("behind no host: %+v %v", p, err)
	}
}
