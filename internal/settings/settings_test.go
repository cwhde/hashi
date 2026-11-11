package settings

import (
	"context"
	"errors"
	"strings"
	"testing"

	"git.juzo.io/juzo/hashi/internal/store/storetest"
)

func validGeneral() General {
	return General{RootDomain: "example.com", AdminDomain: "hashi.example.com", InternalURL: "http://192.0.2.10:3340",
		SyncIntervalSeconds: 3600, TrustedProxies: []string{"192.0.2.0/24"}}
}

func TestGeneralValidate(t *testing.T) {
	if err := validGeneral().Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (General{}).Validate(); err == nil {
		t.Fatal("zero sync interval accepted")
	}
	if err := defaults(General{}).Validate(); err != nil {
		t.Fatalf("defaults before setup: %v", err)
	}
	bad := map[string]func(*General){
		"admin outside root":    func(g *General) { g.AdminDomain = "hashi.example.net" },
		"admin suffix trick":    func(g *General) { g.AdminDomain = "evilexample.com" },
		"root single label":     func(g *General) { g.RootDomain = "localhost"; g.AdminDomain = "localhost" },
		"upper case":            func(g *General) { g.AdminDomain = "Hashi.example.com" },
		"trailing dot":          func(g *General) { g.RootDomain = "example.com." },
		"bad label":             func(g *General) { g.AdminDomain = "-x.example.com" },
		"relative internal URL": func(g *General) { g.InternalURL = "hashi:3340" },
		"ftp internal URL":      func(g *General) { g.InternalURL = "ftp://192.0.2.10" },
		"userinfo in URL":       func(g *General) { g.InternalURL = "http://u:p@192.0.2.10" },
		"sync too short":        func(g *General) { g.SyncIntervalSeconds = 10 },
		"bad proxy":             func(g *General) { g.TrustedProxies = []string{"proxy.local"} },
		"only root set":         func(g *General) { g.AdminDomain = ""; g.InternalURL = "" },
	}
	for name, mut := range bad {
		g := validGeneral()
		mut(&g)
		if err := g.Validate(); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: got %v", name, err)
		}
	}
	g := validGeneral()
	g.AdminDomain = g.RootDomain
	if err := g.Validate(); err != nil {
		t.Errorf("admin = root: %v", err)
	}
}

func TestSecurityValidate(t *testing.T) {
	s := defaults(Security{}).(Security)
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	if s.AdminIdle() != MaxAdminIdle {
		t.Fatalf("default idle %s", s.AdminIdle())
	}
	for _, m := range []int{0, 4, 241} {
		s.AdminIdleTimeoutMinutes = m
		if s.Validate() == nil {
			t.Errorf("idle %d accepted", m)
		}
	}
	s = defaults(Security{}).(Security)
	s.BootstrapAllowedCIDRs = nil
	if s.Validate() == nil {
		t.Error("empty bootstrap networks accepted")
	}
}

func TestPutGetAndNotify(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	svc := New(st)
	var seen []string
	svc.Subscribe(func(c Category) { seen = append(seen, c.Key()) })
	if err := svc.Load(ctx); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 12 {
		t.Fatalf("load notified %v", seen)
	}
	if err := Put(ctx, st.Q, validGeneral()); err != nil {
		t.Fatal(err)
	}
	got, err := Get[General](ctx, st.Q)
	if err != nil || got.AdminDomain != "hashi.example.com" || !got.Configured() {
		t.Fatalf("got %+v %v", got, err)
	}
	if err := Put(ctx, st.Q, Appearance{Theme: "neon"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid put: %v", err)
	}
	if a, _ := Get[Appearance](ctx, st.Q); a.Theme != "graphite" {
		t.Fatalf("invalid value was stored: %+v", a)
	}
}

func TestStoredInvalidValueFailsLoad(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	if _, err := st.Writer.ExecContext(ctx, `INSERT INTO app_settings VALUES ('monitoring', '{"interval_seconds":1}', 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := Get[Monitoring](ctx, st.Q); err == nil {
		t.Fatal("invalid stored value loaded")
	}
}

// Theme data is the exception (10-ui.md §2): an unknown theme or an unreadable stored custom theme falls back to
// Graphite instead of breaking every page and mail.
func TestStoredThemeFallsBack(t *testing.T) {
	ctx := context.Background()
	for _, stored := range []string{
		`{"theme":"neon"}`,
		`{"theme":"custom","custom_theme":{"canvas":`,
		`{"theme":"custom"}`,
		`{"theme":"custom","custom_theme":{"canvas":"#000000","surface":"#111111","text":"#222222","muted":"#333333","border":"#444444","accent":"#555555","roundness":"default"}}`,
	} {
		st := storetest.New(t)
		if _, err := st.Writer.ExecContext(ctx, `INSERT INTO app_settings VALUES ('appearance', ?, 0)`, stored); err != nil {
			t.Fatal(err)
		}
		a, err := Get[Appearance](ctx, st.Q)
		if err != nil || a.Theme != "graphite" || a.CustomTheme != nil {
			t.Errorf("%s: %+v %v", stored, a, err)
		}
	}
}

func TestCustomTheme(t *testing.T) {
	ok := CustomTheme{Canvas: "#0b0d10", Surface: "#14171c", Text: "#eef1f4", Muted: "#8e98a6", Border: "#2a313b",
		Accent: "#14b8a6", Roundness: RoundDefault}
	if err := (Appearance{Theme: ThemeCustom, CustomTheme: &ok}).Validate(); err != nil {
		t.Fatal(err)
	}
	if ok.AccentText() != "#000000" || ok.Light() {
		t.Fatalf("derived %s %v", ok.AccentText(), ok.Light())
	}
	faint := ok
	faint.Muted = "#3a404a"
	if err := faint.Validate(); err == nil || !strings.Contains(err.Error(), "muted text on the surface is too faint") {
		t.Fatalf("faint muted text: %v", err)
	}
	for _, bad := range []func(*CustomTheme){
		func(c *CustomTheme) { c.Accent = "teal" },
		func(c *CustomTheme) { c.Roundness = "blobby" },
		func(c *CustomTheme) {
			c.Charts = []string{"#000000", "#000000", "#000000", "#000000", "#000000", "#000000"}
		},
		func(c *CustomTheme) { c.Text = c.Surface },
	} {
		c := ok
		bad(&c)
		if c.Validate() == nil {
			t.Errorf("accepted %+v", c)
		}
	}
	if (Appearance{Theme: ThemeCustom}).Validate() == nil {
		t.Fatal("custom without a theme accepted")
	}
	// Same numbers as web/src/lib/design/contrast.spec.ts.
	if r := Contrast("#777777", "#ffffff"); r < 4.47 || r > 4.49 {
		t.Fatalf("contrast %v", r)
	}
}
