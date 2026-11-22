package monitor

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/services"
	"git.juzo.io/juzo/hashi/internal/settings"
)

// names lists the layout as "Group: a, b" lines.
func names(l Layout) []string {
	var out []string
	for _, g := range l.Groups {
		var n []string
		for _, e := range g.Entries {
			n = append(n, e.Name)
		}
		out = append(out, g.Name+": "+strings.Join(n, ", "))
	}
	return out
}

func mustLayout(t *testing.T, s *Service) Layout {
	t.Helper()
	l, err := s.Layout(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func keyOf(t *testing.T, l Layout, name string) string {
	t.Helper()
	for _, g := range l.Groups {
		for _, e := range g.Entries {
			if e.Name == name {
				return e.Key
			}
		}
	}
	t.Fatalf("%s not in the layout %v", name, names(l))
	return ""
}

// The first sync fills the layout from what came before (07 §7 Migration): groups in the old category order, then by
// name; entries by name; a service without a check under "Services"; a hidden category's entries off the status page.
func TestLayoutSeedsFromOldGroups(t *testing.T) {
	s, st, _, _ := fixture(t)
	ctx := t.Context()
	if err := settings.Put(ctx, st.Q, settings.StatusPage{Grouped: true, Order: []string{"Media", "Home"}, Hidden: []string{"Lab"}}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ name, group string }{{"Sonarr", "Media"}, {"Jellyfin", "Media"}, {"Lamp", "Home"}, {"Probe", "Lab"}} {
		id, err := s.CreateUser(ctx, c.name, c.group, CheckSpec{Type: TypeTCP, Target: "127.0.0.1:1"})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Update(ctx, id, Settings{Group: c.group, IntervalSeconds: 60, TimeoutSeconds: 5, Public: true}); err != nil {
			t.Fatal(err)
		}
	}
	// The user monitors above seeded the layout already (any write syncs it); start over as a fresh upgrade would.
	if _, err := st.Writer.ExecContext(ctx, `DELETE FROM public_entries; DELETE FROM public_groups; UPDATE public_layout SET seeded = 0, version = 0`); err != nil {
		t.Fatal(err)
	}
	if _, err := services.Create(ctx, st.Q, services.Service{Name: "Wiki", Slug: "wiki", Kind: services.KindTCP, DomainMode: services.ModeNone,
		Enabled: true, Origin: services.OriginUser, PublicPort: 2222, Target: services.Target{Address: "10.0.0.5", Port: 80}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := s.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	l := mustLayout(t, s)
	want := []string{"Media: Jellyfin, Sonarr", "Home: Lamp", "Hashi: Hashi", "Lab: Probe", "Services: Wiki", "Other: "}
	if got := names(l); !slices.Equal(got, want) {
		t.Fatalf("layout\n got %q\nwant %q", got, want)
	}
	if l.Version != 1 {
		t.Fatalf("version %d, want 1 after seeding", l.Version)
	}
	for _, g := range l.Groups {
		for _, e := range g.Entries {
			if e.Name == "Probe" && e.Status {
				t.Fatal("a hidden category's entry stays on the status page")
			}
			if e.Name == "Wiki" && (e.CanApps || e.CanStatus || e.Type != "tcp") {
				t.Fatalf("Wiki %+v: a TCP service without a check has neither switch", e)
			}
		}
	}
}

// Every subject is in the layout exactly once and the layout follows them: a new one joins the end of "Other", a
// removed one leaves, the endpoint's Group field moves it (07 §7).
func TestLayoutFollowsItsSubjects(t *testing.T) {
	s, _, _, _ := fixture(t)
	ctx := t.Context()
	a, err := s.CreateUser(ctx, "Alpha", "Lab", CheckSpec{Type: TypeTCP, Target: "127.0.0.1:1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser(ctx, "Beta", "Lab", CheckSpec{Type: TypeTCP, Target: "127.0.0.1:2"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := services.Create(ctx, s.st.Q, services.Service{Name: "Zed", Slug: "zed", Kind: services.KindTCP, DomainMode: services.ModeNone,
		Enabled: true, Origin: services.OriginUser, PublicPort: 2222, Target: services.Target{Address: "10.0.0.6", Port: 22}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := s.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if got := names(mustLayout(t, s)); !slices.Equal(got, []string{"Lab: Alpha, Beta", "Other: Hashi, Zed"}) {
		t.Fatalf("a new service joins the end of Other: %q", got)
	}
	if err := s.Update(ctx, a, Settings{Group: "Home", IntervalSeconds: 60, TimeoutSeconds: 5}); err != nil {
		t.Fatal(err)
	}
	if got := names(mustLayout(t, s)); !slices.Equal(got, []string{"Lab: Beta", "Home: Alpha", "Other: Hashi, Zed"}) {
		t.Fatalf("the Group field moves the entry: %q", got)
	}
	if err := s.DeleteUser(ctx, a); err != nil {
		t.Fatal(err)
	}
	l := mustLayout(t, s)
	if got := names(l); !slices.Equal(got, []string{"Lab: Beta", "Home: ", "Other: Hashi, Zed"}) {
		t.Fatalf("a removed subject leaves: %q", got)
	}
	seen := map[string]bool{}
	for _, g := range l.Groups {
		for _, e := range g.Entries {
			if seen[e.Key] {
				t.Fatalf("%s twice", e.Key)
			}
			seen[e.Key] = true
		}
	}
}

// A save stores the order and the switches in one go; a stale version, an impossible switch, an unknown or repeated
// entry and a bad group name are refused; an entry the save leaves out stays, at the end of "Other" (07 §7, §8.1).
func TestSaveLayout(t *testing.T) {
	s, st, _, _ := fixture(t)
	ctx := t.Context()
	for _, n := range []string{"Alpha", "Beta"} {
		if _, err := s.CreateUser(ctx, n, "Lab", CheckSpec{Type: TypeTCP, Target: "127.0.0.1:1"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := services.Create(ctx, st.Q, services.Service{Name: "Zed", Slug: "zed", Kind: services.KindTCP, DomainMode: services.ModeNone,
		Enabled: true, Origin: services.OriginUser, PublicPort: 2222, Target: services.Target{Address: "10.0.0.6", Port: 22}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := s.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	l := mustLayout(t, s)
	alpha, beta, zed, hashi := keyOf(t, l, "Alpha"), keyOf(t, l, "Beta"), keyOf(t, l, "Zed"), keyOf(t, l, "Hashi")
	lab := l.Groups[0].ID // Lab
	yes := true
	bad := []struct {
		why string
		in  LayoutSave
	}{
		{"stale", LayoutSave{Version: l.Version - 1}},
		{"Zed has no web address", LayoutSave{Version: l.Version, Switches: []LayoutSwitch{{Entry: zed, Apps: &yes}}}},
		{"unknown entry", LayoutSave{Version: l.Version, Other: []string{"service:nope"}}},
		{"twice", LayoutSave{Version: l.Version, Other: []string{alpha, alpha}}},
		{"Other", LayoutSave{Version: l.Version, Groups: []LayoutSaveGroup{{Name: "other"}}}},
		{"two groups", LayoutSave{Version: l.Version, Groups: []LayoutSaveGroup{{Name: "Lab"}, {Name: "lab"}}}},
		{"empty name", LayoutSave{Version: l.Version, Groups: []LayoutSaveGroup{{Name: " "}}}},
	}
	for _, b := range bad {
		if _, err := s.SaveLayout(ctx, b.in); err == nil {
			t.Fatalf("%s: saved", b.why)
		} else if b.why == "stale" && !errors.Is(err, ErrLayoutStale) {
			t.Fatalf("stale: %v", err)
		}
	}
	got, err := s.SaveLayout(ctx, LayoutSave{Version: l.Version,
		Groups:   []LayoutSaveGroup{{ID: lab, Name: "Tools", Entries: []string{zed, beta}}, {Name: "Core", Entries: []string{hashi}}},
		Switches: []LayoutSwitch{{Entry: beta, Status: &yes}}})
	if err != nil {
		t.Fatal(err)
	}
	if n := names(got); !slices.Equal(n, []string{"Tools: Zed, Beta", "Core: Hashi", "Other: Alpha"}) {
		t.Fatalf("saved layout %q (Alpha was left out and stays in Other)", n)
	}
	if got.Version != l.Version+1 || got.Groups[0].ID != lab {
		t.Fatalf("version %d, group id %q: the version moves on and a renamed group keeps its id", got.Version, got.Groups[0].ID)
	}
	ep, err := st.R.GetMonitorEndpoint(ctx, strings.TrimPrefix(beta, EntryEndpoint+":"))
	if err != nil {
		t.Fatal(err)
	}
	if ep.Public != 1 || ep.GroupName != "Tools" {
		t.Fatalf("Beta public %d group %q: the switch and the group reach the endpoint", ep.Public, ep.GroupName)
	}
	if err := s.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if again := mustLayout(t, s); again.Version != got.Version {
		t.Fatalf("a sync with nothing new changed the version %d → %d", got.Version, again.Version)
	}
}

func TestSlugify(t *testing.T) {
	for in, want := range map[string]string{"Home Assistant": "home-assistant", "  qBittorrent!": "qbittorrent", "Ñ": "entry", "a--b": "a-b"} {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}
