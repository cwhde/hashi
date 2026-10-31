package ipsets

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"net/netip"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)

type fakeResolver struct {
	hosts map[string][]netip.Addr
	dns   map[string][]netip.Addr
	fail  map[string]bool
}

func (f fakeResolver) Pulse(id, scope, selector string) ([]netip.Addr, error) {
	return f.Host("pulse:"+id, scope)
}

func (f fakeResolver) Host(id, scope string) ([]netip.Addr, error) {
	if f.fail[id] {
		return nil, errors.New("unreachable")
	}
	return f.hosts[id+"/"+scope], nil
}

func (f fakeResolver) DNS(n string) ([]netip.Addr, error) {
	if f.fail[n] {
		return nil, errors.New("SERVFAIL")
	}
	return f.dns[n], nil
}

func el(id string, it Item) Element { return Element{ID: id, Item: it} }

func mustItem(t *testing.T, s string) Item {
	t.Helper()
	it, err := ParseIPOrCIDR(s)
	if err != nil {
		t.Fatal(err)
	}
	return it
}

func graph(sets ...*Set) Graph {
	g := Graph{}
	for _, s := range sets {
		if s.Kind == "" {
			s.Kind = KindUser
		}
		if s.Slug == "" {
			s.Slug = s.ID
		}
		g[s.ID] = s
	}
	return g
}

func TestParseIPOrCIDRNormalizes(t *testing.T) {
	cases := map[string]string{
		"192.0.2.7":            "ip 192.0.2.7",
		" 192.0.2.7 ":          "ip 192.0.2.7",
		"192.0.2.7/32":         "ip 192.0.2.7",
		"192.0.2.77/24":        "cidr 192.0.2.0/24",
		"::ffff:192.0.2.7":     "ip 192.0.2.7",
		"::ffff:192.0.2.0/120": "cidr 192.0.2.0/24",
		"2001:DB8::1":          "ip 2001:db8::1",
		"2001:db8::1/128":      "ip 2001:db8::1",
		"2001:db8:1:2::5/48":   "cidr 2001:db8:1::/48",
		"0.0.0.0/0":            "cidr 0.0.0.0/0",
	}
	for in, want := range cases {
		it, err := ParseIPOrCIDR(in)
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		got := it.Type() + " "
		switch v := it.(type) {
		case IP:
			got += v.Addr.String()
		case CIDR:
			got += v.Prefix.String()
		}
		if got != want {
			t.Errorf("%q → %q, want %q", in, got, want)
		}
	}
	for _, bad := range []string{"", "192.0.2", "192.0.2.1/33", "fe80::1%eth0", "::ffff:0:0/90", "host.example"} {
		if _, err := ParseIPOrCIDR(bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("%q: err %v, want ErrInvalid", bad, err)
		}
	}
}

func TestNormalizeFQDN(t *testing.T) {
	if n, err := NormalizeFQDN(" Home.Example.COM. "); err != nil || n != "home.example.com" {
		t.Fatalf("got %q %v", n, err)
	}
	for _, bad := range []string{"localhost", "-a.example.com", "a..example.com", "192.0.2.1", "a b.example.com", strings.Repeat("a", 64) + ".example.com"} {
		if _, err := NormalizeFQDN(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestNormalizeRejectsBadFields(t *testing.T) {
	for _, it := range []Item{nil, IP{}, CIDR{}, Host{HostID: "h"}, Host{Scope: ScopePublic}, DNS{FQDN: "x"}, SetRef{}} {
		if _, err := Normalize(it); !errors.Is(err, ErrInvalid) {
			t.Errorf("%#v: err %v", it, err)
		}
	}
}

func TestInvariant_22_SetGraphsAcyclic(t *testing.T) {
	a := &Set{ID: "a", Elements: []Element{el("1", SetRef{SetID: "b"})}}
	b := &Set{ID: "b", Elements: []Element{el("2", SetRef{SetID: "c"})}}
	c := &Set{ID: "c"}
	g := graph(a, b, c)

	var ce *CycleError
	if err := g.CheckReference("c", "a"); !errors.As(err, &ce) || strings.Join(ce.Path, " → ") != "c → a → b → c" {
		t.Fatalf("indirect: %v", err)
	}
	if err := g.CheckReference("b", "a"); !errors.As(err, &ce) || strings.Join(ce.Path, " → ") != "b → a → b" {
		t.Fatalf("direct: %v", err)
	}
	if err := g.CheckReference("a", "a"); !errors.As(err, &ce) || strings.Join(ce.Path, " → ") != "a → a" {
		t.Fatalf("self: %v", err)
	}
	if !errors.Is(g.CheckReference("a", "a"), ErrInvalid) {
		t.Fatal("cycle error must be ErrInvalid (400)")
	}
	if err := g.CheckReference("a", "c"); err != nil {
		t.Fatalf("diamond is not a cycle: %v", err)
	}

	// A cycle in stored data (it can't be saved, but compile must still terminate) becomes a problem.
	c.Elements = []Element{el("3", SetRef{SetID: "a"}), el("4", mustItem(t, "192.0.2.1"))}
	res := g.Compile("a", nil, now)
	if res.Status != StatusPartial || len(res.Problems) != 1 || !strings.Contains(res.Problems[0].Message, "cycle") {
		t.Fatalf("stored cycle: %+v", res)
	}
}

func TestCompileUnionMergeAndSplit(t *testing.T) {
	inner := &Set{ID: "inner", Elements: []Element{
		el("i1", mustItem(t, "192.0.2.0/25")),
		el("i2", mustItem(t, "2001:db8::/33")),
	}}
	outer := &Set{ID: "outer", Elements: []Element{
		el("o1", mustItem(t, "192.0.2.128/25")),
		el("o2", mustItem(t, "192.0.2.5")),
		el("o3", SetRef{SetID: "inner"}),
		el("o4", mustItem(t, "2001:db8:8000::/33")),
		el("o5", mustItem(t, "198.51.100.1")),
	}}
	res := graph(inner, outer).Compile("outer", nil, now)
	if fmt.Sprint(res.V4) != "[192.0.2.0/24 198.51.100.1/32]" || fmt.Sprint(res.V6) != "[2001:db8::/32]" {
		t.Fatalf("v4 %v v6 %v", res.V4, res.V6)
	}
	if res.Status != StatusOK || res.AddressSet {
		t.Fatalf("status %s address set %v", res.Status, res.AddressSet)
	}
	if !res.Contains(netip.MustParseAddr("::ffff:192.0.2.200")) || res.Contains(netip.MustParseAddr("203.0.113.1")) {
		t.Fatal("Contains")
	}
}

func TestCompileStatusAndProblems(t *testing.T) {
	r := fakeResolver{
		hosts: map[string][]netip.Addr{"h1/public": {netip.MustParseAddr("203.0.113.10")}},
		dns:   map[string][]netip.Addr{"home.example.com": {netip.MustParseAddr("198.51.100.20"), netip.MustParseAddr("2001:db8::20")}},
		fail:  map[string]bool{"down.example.com": true},
	}
	child := &Set{ID: "child", Slug: "child", Elements: []Element{el("c1", DNS{FQDN: "down.example.com"})}}
	s := &Set{ID: "s", Slug: "s", Elements: []Element{
		el("1", Host{HostID: "h1", Scope: ScopePublic}),
		el("2", DNS{FQDN: "home.example.com"}),
		el("3", SetRef{SetID: "child"}),
		el("4", DNS{FQDN: "none.example.com"}),
	}}
	res := graph(s, child).Compile("s", r, now)
	if res.Status != StatusPartial || !res.AddressSet {
		t.Fatalf("status %s address set %v", res.Status, res.AddressSet)
	}
	if fmt.Sprint(res.Addresses) != "[203.0.113.10 198.51.100.20 2001:db8::20]" {
		t.Fatalf("addresses in item order: %v", res.Addresses)
	}
	if len(res.Problems) != 2 || strings.Join(res.Problems[0].Path, " → ") != "s → child" ||
		res.Problems[0].ElementID != "c1" || !strings.Contains(res.Problems[1].Message, "no address") {
		t.Fatalf("problems %+v", res.Problems)
	}
	if empty := graph(child).Compile("child", r, now); empty.Status != StatusEmpty {
		t.Fatalf("only failing items → empty, got %s", empty.Status)
	}
	if missing := graph(&Set{ID: "m", Elements: []Element{el("x", SetRef{SetID: "gone"})}}).Compile("m", r, now); missing.Status != StatusEmpty || len(missing.Problems) != 1 {
		t.Fatalf("missing reference: %+v", missing)
	}
}

func TestCompileDropsExpired(t *testing.T) {
	past, future := now.Add(-time.Second), now.Add(time.Hour)
	s := &Set{ID: "s", Elements: []Element{
		{ID: "1", Item: mustItem(t, "192.0.2.1"), ExpiresAt: &past},
		{ID: "2", Item: mustItem(t, "192.0.2.2"), ExpiresAt: &future},
		{ID: "3", Item: mustItem(t, "192.0.2.3"), ExpiresAt: &now},
	}}
	if res := graph(s).Compile("s", nil, now); fmt.Sprint(res.V4) != "[192.0.2.2/32]" {
		t.Fatalf("got %v", res.V4)
	}
}

func TestAddressSetCapabilityThroughNesting(t *testing.T) {
	leaf := &Set{ID: "leaf", Elements: []Element{el("1", mustItem(t, "192.0.2.1"))}}
	mid := &Set{ID: "mid", Elements: []Element{el("2", SetRef{SetID: "leaf"}), el("3", DNS{FQDN: "a.example.com"})}}
	top := &Set{ID: "top", Elements: []Element{el("4", SetRef{SetID: "mid"})}}
	g := graph(leaf, mid, top)
	if !g.Compile("top", nil, now).AddressSet {
		t.Fatal("ip + dns through nesting is an address set")
	}
	leaf.Elements = append(leaf.Elements, el("5", mustItem(t, "192.0.2.0/24")))
	if g.Compile("top", nil, now).AddressSet {
		t.Fatal("a cidr two levels down makes it a match-only set")
	}
	feed := &Set{ID: "feed", Kind: KindFeed, Elements: []Element{el("6", mustItem(t, "192.0.2.9"))}}
	g["feed"] = feed
	top.Elements = []Element{el("7", SetRef{SetID: "feed"})}
	if g.Compile("top", nil, now).AddressSet {
		t.Fatal("feed sets are never address sets")
	}
}

func TestCompileDeepNesting(t *testing.T) {
	g := Graph{}
	const depth = 200
	for i := range depth {
		s := &Set{ID: fmt.Sprint(i), Slug: fmt.Sprint(i), Kind: KindUser,
			Elements: []Element{el(fmt.Sprint("e", i), IP{Addr: netip.AddrFrom4([4]byte{10, 0, byte(i / 256), byte(i)})})}}
		if i+1 < depth {
			s.Elements = append(s.Elements, el(fmt.Sprint("r", i), SetRef{SetID: fmt.Sprint(i + 1)}))
		}
		g[s.ID] = s
	}
	if res := g.Compile("0", nil, now); res.Status != StatusOK || len(res.Addresses) != depth {
		t.Fatalf("status %s, %d addresses", res.Status, len(res.Addresses))
	}
}

// The merged output covers exactly the union of the inputs: checked against a brute-force membership test.
func TestCompileMergeProperty(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	for round := range 200 {
		var els []Element
		var in []netip.Prefix
		for i := range 1 + rng.IntN(12) {
			a := netip.AddrFrom4([4]byte{10, byte(rng.IntN(4)), byte(rng.IntN(256)), byte(rng.IntN(256))})
			p := netip.PrefixFrom(a, 14+rng.IntN(19)).Masked()
			in = append(in, p)
			it, err := ParseIPOrCIDR(p.String())
			if err != nil {
				t.Fatal(err)
			}
			els = append(els, el(fmt.Sprint(i), it))
		}
		res := graph(&Set{ID: "s", Elements: els}).Compile("s", nil, now)
		for i := 1; i < len(res.V4); i++ {
			if res.V4[i-1].Overlaps(res.V4[i]) || !res.V4[i-1].Addr().Less(res.V4[i].Addr()) {
				t.Fatalf("round %d: output not sorted and disjoint: %v", round, res.V4)
			}
		}
		for range 500 {
			x := netip.AddrFrom4([4]byte{10, byte(rng.IntN(4)), byte(rng.IntN(256)), byte(rng.IntN(256))})
			want := false
			for _, p := range in {
				want = want || p.Contains(x)
			}
			if res.Contains(x) != want {
				t.Fatalf("round %d: %s in output %v, in input %v", round, x, res.Contains(x), want)
			}
		}
	}
}

func TestResultHashChangesWithContent(t *testing.T) {
	a := Result{V4: []netip.Prefix{netip.MustParsePrefix("192.0.2.0/24")}}
	b := Result{V4: []netip.Prefix{netip.MustParsePrefix("192.0.2.0/25")}}
	if a.Hash() == b.Hash() || a.Hash() != (Result{V4: a.V4}).Hash() {
		t.Fatal("hash")
	}
}

// A stale item keeps its addresses and marks the set partial (05a §2).
func TestStaleKeepsAddressesAndWarns(t *testing.T) {
	g := Graph{"s": {ID: "s", Slug: "s", Elements: []Element{{ID: "e", Item: Pulse{AgentID: "a", Scope: ScopeInternal}}}}}
	res := g.Compile("s", staleResolver{}, time.Now())
	if res.Status != StatusPartial || len(res.Addresses) != 1 || len(res.Problems) != 1 {
		t.Fatalf("%+v", res)
	}
}

type staleResolver struct{ fakeResolver }

func (staleResolver) Pulse(string, string, string) ([]netip.Addr, error) {
	return []netip.Addr{netip.MustParseAddr("10.0.0.1")}, Stale{What: "stale"}
}
