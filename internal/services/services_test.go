package services

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/store"
	"git.juzo.io/juzo/hashi/internal/store/db"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
	"git.juzo.io/juzo/hashi/internal/vault"
)

var now = time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)

// newStore has the state setup steps 2–3 leave: root example.com, zone example.com.
func newStore(t *testing.T) *store.Store {
	t.Helper()
	ctx := context.Background()
	st := storetest.New(t)
	if err := settings.Put(ctx, st.Q, settings.General{RootDomain: "example.com", AdminDomain: "hashi.example.com",
		InternalURL: "http://192.0.2.5:3340", SyncIntervalSeconds: 3600}); err != nil {
		t.Fatal(err)
	}
	v, err := vault.New(bytes.Repeat([]byte{7}, vault.KeySize), 1)
	if err != nil {
		t.Fatal(err)
	}
	rec, err := v.Seal(vault.Service, "dns.token", secrets.New("t"))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Q.InsertSecret(ctx, rec); err != nil {
		t.Fatal(err)
	}
	if err := st.Q.InsertDNSProvider(ctx, db.InsertDNSProviderParams{ID: "p1", Name: "hetzner", Type: "hetzner",
		DefaultTtl: 3600, TokenSecretID: rec.ID, Health: "healthy", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := st.Q.InsertDNSZone(ctx, db.InsertDNSZoneParams{ID: "z1", ProviderID: "p1", ProviderZoneID: "1",
		Name: "example.com", ZoneTtl: 3600, CreatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	return st
}

func web() Service {
	return Service{Name: "Search", Slug: "search", Kind: KindHTTP, DomainMode: ModeSubdomain, HostName: "search.example.com",
		Enabled: true, Target: Target{Address: "192.0.2.20", Port: 8080}, DashboardVisible: true}
}

func TestCreateListUpdateDelete(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	s := web()
	s.AdvancedRoutes = true
	s.Routes = []Route{{Enabled: true, Targets: []Target{{Address: "192.0.2.21", Port: 9000, Weight: 2}},
		PathMatch: MatchPrefix, PathValue: "/api", Priority: 10, RewriteMode: RewriteStripPrefix}}
	created, err := Create(ctx, st.Q, s, now)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Get(ctx, st.Q, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Origin != OriginUser || got.InvalidReason != "" || len(got.Routes) != 1 || got.Routes[0].ID == "" ||
		got.Routes[0].Targets[0].Weight != 2 || !got.Renderable() {
		t.Fatalf("got %+v", got)
	}
	routeID := got.Routes[0].ID

	got.Target.Port = 8083
	got.Enabled = false // ignored by Update: enable/disable has its own call
	if _, err := Update(ctx, st.Q, got, now); err != nil {
		t.Fatal(err)
	}
	again, _ := Get(ctx, st.Q, created.ID)
	if again.Target.Port != 8083 || !again.Enabled || again.Routes[0].ID != routeID {
		t.Errorf("after update: %+v", again)
	}
	if _, err := SetEnabled(ctx, st.Q, created.ID, false, now); err != nil {
		t.Fatal(err)
	}
	if again, _ = Get(ctx, st.Q, created.ID); again.Renderable() {
		t.Error("disabled service still renderable")
	}

	dup := web()
	dup.Slug = "other"
	if _, err := Create(ctx, st.Q, dup, now); !errors.Is(err, ErrTaken) {
		t.Errorf("same host name: %v", err)
	}
	if _, err := Delete(ctx, st.Q, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := Get(ctx, st.Q, created.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("after delete: %v", err)
	}
}

// The Hashi admin service follows General settings and cannot be edited, disabled or deleted through the service API.
func TestInvariant_10_SystemServiceOnlyThroughSettings(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	s, changed, err := EnsureSystem(ctx, st.Q, now)
	if err != nil || !changed {
		t.Fatalf("%v %v", changed, err)
	}
	if s.HostName != "hashi.example.com" || s.Target != (Target{Address: "192.0.2.5", Port: 3340}) || s.Kind != KindHTTP ||
		!s.System() || s.OwnerFeature != OwnerSetup {
		t.Fatalf("got %+v", s)
	}
	if _, changed, _ = EnsureSystem(ctx, st.Q, now); changed {
		t.Error("second ensure changed something")
	}
	if _, err := Update(ctx, st.Q, s, now); !errors.Is(err, ErrSystem) {
		t.Errorf("update: %v", err)
	}
	if _, err := SetEnabled(ctx, st.Q, s.ID, false, now); !errors.Is(err, ErrSystem) {
		t.Errorf("disable: %v", err)
	}
	if _, err := Delete(ctx, st.Q, s.ID); !errors.Is(err, ErrSystem) {
		t.Errorf("delete: %v", err)
	}

	if err := settings.Put(ctx, st.Q, settings.General{RootDomain: "example.com", AdminDomain: "dev.hashi.example.com",
		InternalURL: "https://hashi.internal.example.com", SyncIntervalSeconds: 3600}); err != nil {
		t.Fatal(err)
	}
	s2, changed, err := EnsureSystem(ctx, st.Q, now)
	if err != nil || !changed || s2.ID != s.ID || s2.HostName != "dev.hashi.example.com" ||
		s2.Target != (Target{Address: "hashi.internal.example.com", Port: 443}) || s2.Kind != KindHTTPS {
		t.Fatalf("after settings change: %+v %v %v", s2, changed, err)
	}
}

func TestValidate(t *testing.T) {
	n := Names{Root: "example.com", Zone: "example.com"}
	route := func(mut func(*Route)) Service {
		s := web()
		s.AdvancedRoutes = true
		r := Route{Enabled: true, Targets: []Target{{Address: "192.0.2.30", Port: 80}}, PathMatch: MatchPrefix,
			PathValue: "/x", Priority: 5, RewriteMode: RewriteNone}
		mut(&r)
		s.Routes = []Route{r}
		return s
	}
	bad := map[string]Service{
		"outside zone":  func() Service { s := web(); s.HostName = "search.example.net"; return s }(),
		"not a sub":     func() Service { s := web(); s.HostName = "example.com"; return s }(),
		"root mismatch": func() Service { s := web(); s.DomainMode = ModeRoot; return s }(),
		"upper case":    func() Service { s := web(); s.HostName = "Search.example.com"; return s }(),
		"on label":      func() Service { s := web(); s.HostName = "x.on.example.com"; return s }(),
		"via label":     func() Service { s := web(); s.HostName = "via.example.com"; return s }(),
		"ovr label":     func() Service { s := web(); s.HostName = "a.ovr.b.example.com"; return s }(),
		"challenge":     func() Service { s := web(); s.HostName = "_acme-challenge.example.com"; return s }(),
		"port zero":     func() Service { s := web(); s.Target.Port = 0; return s }(),
		"bad address":   func() Service { s := web(); s.Target.Address = "192.0.2.30;x"; return s }(),
		"zoned v6":      func() Service { s := web(); s.Target.Address = "fe80::1%eth0"; return s }(),
		"tls on http":   func() Service { s := web(); s.TLSSkipVerify = true; return s }(),
		"slug":          func() Service { s := web(); s.Slug = "Search!"; return s }(),
		"min tls":       func() Service { s := web(); s.TLSMinVersion = "VersionTLS10"; return s }(),
		"no slash":      route(func(r *Route) { r.PathValue = "x" }),
		"bad regex":     route(func(r *Route) { r.PathMatch, r.PathValue = MatchRegex, "(" }),
		"backtick":      route(func(r *Route) { r.PathValue = "/a`)||Host(`evil" }),
		"template":      route(func(r *Route) { r.RewriteValue, r.RewriteMode = `/{{ env "HOME" }}`, RewriteExact }),
		"no targets":    route(func(r *Route) { r.Targets = nil }),
		"strip regex":   route(func(r *Route) { r.PathMatch, r.PathValue, r.RewriteMode = MatchRegex, "^/a", RewriteStripPrefix }),
		"rewrite empty": route(func(r *Route) { r.RewriteMode = RewriteRegex; r.RewritePattern = "^/x(.*)" }),
		"value no mode": route(func(r *Route) { r.RewriteValue = "/y" }),
		"priority":      route(func(r *Route) { r.Priority = 0 }),
		"routes off, bad route": func() Service {
			s := route(func(r *Route) { r.PathMatch = "glob" })
			s.AdvancedRoutes = false
			return s
		}(),
		"routes off, same priority": func() Service {
			s := route(func(*Route) {})
			s.Routes = append(s.Routes, s.Routes[0])
			s.AdvancedRoutes = false
			return s
		}(),
	}
	for name, s := range bad {
		var ie *InvalidError
		if err := Validate(s, n); !errors.As(err, &ie) {
			t.Errorf("%s accepted: %v", name, err)
		}
	}
	good := []Service{
		web(),
		func() Service { s := web(); s.HostName = "dev.hashi.example.com"; return s }(),
		func() Service { s := web(); s.DomainMode, s.HostName = ModeRoot, "example.com"; return s }(),
		func() Service { s := web(); s.Target.Address = "2001:db8::5"; return s }(),
		func() Service { s := web(); s.Target.Address = "localhost"; return s }(),
		func() Service {
			s := web()
			s.Kind, s.TLSSkipVerify, s.TLSServerName, s.TLSMinVersion = KindHTTPS, true, "app.internal.example", TLS13
			return s
		}(),
		route(func(r *Route) { r.RewriteMode, r.RewriteValue = RewriteReplacePrefix, "/y" }),
		route(func(r *Route) {
			r.PathMatch, r.PathValue, r.RewriteMode, r.RewriteValue = MatchExact, "/a", RewriteExact, "/b"
		}),
		route(func(r *Route) {
			r.PathMatch, r.PathValue = MatchRegex, `^/u/(\d+)`
			r.RewriteMode, r.RewritePattern, r.RewriteValue = RewriteRegex, `^/u/(\d+)`, "/user/$1"
		}),
	}
	for i, s := range good {
		if err := Validate(s, n); err != nil {
			t.Errorf("good[%d]: %v", i, err)
		}
	}
	// A deeper root: custom names elsewhere in the zone are allowed, reserved labels apply under the root only.
	deep := Names{Root: "home.example.com", Zone: "example.com"}
	s := web()
	s.DomainMode, s.HostName = ModeCustom, "on.example.com"
	if err := Validate(s, deep); err != nil {
		t.Errorf("custom name outside the root: %v", err)
	}
}

// A service that stops validating (the zone moved) is listed with its reason and left out of rendering.
func TestListMarksInvalid(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	if _, err := Create(ctx, st.Q, web(), now); err != nil {
		t.Fatal(err)
	}
	if err := settings.Put(ctx, st.Q, settings.General{RootDomain: "home.example.com", AdminDomain: "hashi.home.example.com",
		InternalURL: "http://192.0.2.5:3340", SyncIntervalSeconds: 3600}); err != nil {
		t.Fatal(err)
	}
	all, err := List(ctx, st.Q)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].Renderable() || !strings.Contains(all[0].InvalidReason, "subdomain of home.example.com") {
		t.Errorf("got %+v", all)
	}
}
