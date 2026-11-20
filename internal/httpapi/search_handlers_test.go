package httpapi

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/auth"
	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
	"git.juzo.io/juzo/hashi/internal/ipsets"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
)

// Bug 4: the top bar search found only page names. It now finds what Hashi stores, and an address finds the IP set items
// that hold it.
func TestSearchFindsStoredObjects(t *testing.T) {
	ctx := context.Background()
	st := storetest.Open(t, t.TempDir()+"/h.db")
	al := audit.New([]byte("k"))
	s := &Server{Store: st, Audit: al, Sessions: auth.NewManager(st, al, []byte("csrf")),
		IPSets: ipsets.NewService(st, al, slog.New(slog.NewTextHandler(io.Discard, nil)), nil), Log: slog.Default()}
	created, err := s.CreateIPSet(ctx, gen.CreateIPSetRequestObject{Body: &gen.IPSetInput{Slug: "trusted-devices", Name: "Trusted devices"}})
	if err != nil {
		t.Fatal(err)
	}
	set := created.(gen.CreateIPSet200JSONResponse)
	if _, err := s.AddIPSetElement(ctx, gen.AddIPSetElementRequestObject{Id: set.Id,
		Body: &gen.IPSetElementInput{Item: ipItem(t, "cidr", "198.51.100.0/24")}}); err != nil {
		t.Fatal(err)
	}
	search := func(q string) gen.SearchResults {
		t.Helper()
		r, err := s.Search(ctx, gen.SearchRequestObject{Params: gen.SearchParams{Q: q}})
		if err != nil {
			t.Fatal(err)
		}
		return gen.SearchResults(r.(gen.Search200JSONResponse))
	}
	group := func(r gen.SearchResults, k gen.SearchGroupKind) *gen.SearchGroup {
		for i := range r.Groups {
			if r.Groups[i].Kind == k {
				return &r.Groups[i]
			}
		}
		return nil
	}

	byName := search("trusted DEV")
	g := group(byName, gen.SearchGroupKindIpSet)
	if g == nil || len(g.Results) != 1 || g.Results[0].Title != "Trusted devices" || g.Results[0].Href != "/ip-sets/"+set.Id {
		t.Fatalf("by name: %+v", byName)
	}
	if byName.Address != nil {
		t.Errorf("a name is not an address: %q", *byName.Address)
	}

	byAddr := search("198.51.100.7")
	if byAddr.Address == nil || *byAddr.Address != "198.51.100.7" {
		t.Fatalf("address: %+v", byAddr.Address)
	}
	g = group(byAddr, gen.SearchGroupKindIpSetItem)
	if g == nil || len(g.Results) != 1 || g.Results[0].Title != "198.51.100.0/24" || g.Results[0].Detail != "in Trusted devices" {
		t.Fatalf("items holding the address: %+v", byAddr)
	}
	if g.Results[0].Href != "/ip-sets/"+set.Id+"?q=198.51.100.7" {
		t.Errorf("item href = %q", g.Results[0].Href)
	}

	if none := search("nothing-like-this"); len(none.Groups) != 0 {
		t.Errorf("no match: %+v", none)
	}
}

func TestSearchRanksAndCaps(t *testing.T) {
	q := newSearchQuery("git")
	var hits []searchHit
	for _, n := range []string{"forgejo (old git)", "gitea", "git", "gitlab", "a", "b git", "c git", "d git", "e git", "f git"} {
		hits = append(hits, searchHit{title: n})
	}
	g := q.group(gen.SearchGroupKindService, "Services", hits, false)
	got := []string{}
	for _, r := range g.Results {
		got = append(got, r.Title)
	}
	want := []string{"git", "gitea", "gitlab", "forgejo (old git)", "b git", "c git", "d git", "e git"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	if g.More != 1 {
		t.Errorf("more = %d, want 1 (\"a\" doesn't match)", g.More)
	}
}

// "gitea waf" opens the service's WAF tab; "gitea" alone opens the page.
func TestSearchOpensAnObjectsTab(t *testing.T) {
	hits := []searchHit{{title: "gitea", href: "/services/1"}, {title: "grafana", href: "/services/2"}}
	q := newSearchQuery("gitea waf")
	g := q.group(gen.SearchGroupKindService, "Services", q.withTabs(hits, serviceTabs), false)
	if g == nil || len(g.Results) != 1 || g.Results[0].Title != "gitea · WAF" || g.Results[0].Href != "/services/1?tab=waf" {
		t.Fatalf("%+v", g)
	}
	q = newSearchQuery("gitea")
	g = q.group(gen.SearchGroupKindService, "Services", q.withTabs(hits, serviceTabs), false)
	if g == nil || len(g.Results) != 1 || g.Results[0].Href != "/services/1" {
		t.Fatalf("%+v", g)
	}
}

// The tab lists above copy the pages' tabs; this keeps them true when a page renames or drops one.
func TestSearchTabsExistOnTheirPages(t *testing.T) {
	pages := map[string][]objectTab{
		"services/[id]": serviceTabs, "firewall/[id]": firewallTabs, "ip-sets/[id]": ipSetTabs,
	}
	for page, tabs := range pages {
		src, err := os.ReadFile("../../web/src/routes/(app)/" + page + "/+page.svelte")
		if err != nil {
			t.Fatal(err)
		}
		for _, tab := range tabs {
			if !strings.Contains(string(src), `value="`+tab.value+`"`) || !strings.Contains(string(src), tab.label) {
				t.Errorf("%s has no tab %q labelled %q", page, tab.value, tab.label)
			}
		}
	}
}
