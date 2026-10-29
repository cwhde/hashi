package ipsets

import (
	"fmt"
	"net/netip"
	"strings"
	"testing"
)

func TestExplainAndContaining(t *testing.T) {
	r := fakeResolver{dns: map[string][]netip.Addr{"phone.example.com": {netip.MustParseAddr("198.51.100.7")}}}
	devices := &Set{ID: "d", Slug: "trusted-devices", Elements: []Element{
		{ID: "phone", Item: DNS{FQDN: "phone.example.com"}, Note: "phone"},
		{ID: "lan", Item: mustItem(t, "198.51.100.0/24")},
	}}
	office := &Set{ID: "o", Slug: "office", Elements: []Element{{ID: "ref", Item: SetRef{SetID: "d"}}}}
	other := &Set{ID: "x", Slug: "other", Elements: []Element{{ID: "y", Item: mustItem(t, "203.0.113.1")}}}
	g := graph(devices, office, other)
	a := netip.MustParseAddr("198.51.100.7")

	var got []string
	for _, m := range g.Explain("o", a, r, now) {
		got = append(got, strings.Join(m.Path, " → ")+" : "+Describe(g, m.Item))
	}
	want := "[office → trusted-devices : phone.example.com (dns) office → trusted-devices : 198.51.100.0/24 (cidr)]"
	if fmt.Sprint(got) != want {
		t.Fatalf("got %v", got)
	}
	if ids := g.Containing(a, r, now); fmt.Sprint(ids) != "[o d]" {
		t.Fatalf("containing %v", ids)
	}
}

func TestSearch(t *testing.T) {
	inner := &Set{ID: "i", Slug: "inner", Elements: []Element{{ID: "i1", Item: mustItem(t, "203.0.113.0/24")}}}
	s := &Set{ID: "s", Slug: "s", Elements: []Element{
		{ID: "a", Item: mustItem(t, "192.0.2.5"), Note: "Laptop"},
		{ID: "b", Item: mustItem(t, "192.0.2.0/24")},
		{ID: "c", Item: SetRef{SetID: "i"}},
		{ID: "d", Item: DNS{FQDN: "home.example.com"}},
	}}
	g := graph(s, inner)
	ids := func(q string) string {
		var out []string
		for _, e := range g.Search("s", q, nil, now) {
			out = append(out, e.ID)
		}
		return strings.Join(out, ",")
	}
	for q, want := range map[string]string{
		"":               "a,b,c,d",
		"192.0.2.5":      "a,b",
		"192.0.2.128/25": "b",
		"203.0.113.9":    "c",
		"laptop":         "a",
		"home.example":   "d",
		"inner":          "c",
	} {
		if got := ids(q); got != want {
			t.Errorf("%q: %s, want %s", q, got, want)
		}
	}
}
