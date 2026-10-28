package dns

import (
	"errors"
	"slices"
	"testing"
)

// caps is the Hetzner adapter's capability set (kept here to avoid an import cycle).
var caps = Capabilities{ManagedTypes: []string{"A", "AAAA", "CNAME", "MX", "TXT"}, ReadOnlyTypes: []string{"NS", "SOA"}}

func TestInvariant_2_NSSOANeverSelectableForPrune(t *testing.T) {
	for _, typ := range []string{"NS", "SOA", "ns", "soa"} {
		r := Normalize(RRSet{Name: "@", Type: typ, Values: []string{"x."}})
		if got := AllowedDecisions(r, caps); !slices.Equal(got, []Decision{Leave}) {
			t.Errorf("%s: allowed = %v, want only leave", typ, got)
		}
		for _, d := range []Decision{Prune, Import} {
			if err := CheckDecision(r, caps, d); err == nil {
				t.Errorf("%s: %s accepted", typ, d)
			}
		}
		if err := CheckDecision(r, caps, Leave); err != nil {
			t.Errorf("%s: leave rejected: %v", typ, err)
		}
	}
}

func TestProtectedRRSetOnlyLeave(t *testing.T) {
	r := Normalize(RRSet{Name: "www", Type: "A", Values: []string{"192.0.2.1"}, Protected: true})
	if err := CheckDecision(r, caps, Prune); err == nil {
		t.Error("prune of a change-protected RRSet accepted")
	}
	open := Normalize(RRSet{Name: "www", Type: "A", Values: []string{"192.0.2.1"}})
	for _, d := range []Decision{Import, Leave, Prune} {
		if err := CheckDecision(open, caps, d); err != nil {
			t.Errorf("%s: %v", d, err)
		}
	}
	if err := CheckDecision(open, caps, "delete"); err == nil {
		t.Error("unknown decision accepted")
	}
}

func TestDecodeTXT(t *testing.T) {
	cases := []struct {
		in, want string
		ok       bool
	}{
		{`"v=spf1 -all"`, "v=spf1 -all", true},
		{`"abc" "def"`, "abcdef", true},
		{`"a\"b"`, `a"b`, true},
		{`"a\\b"`, `a\b`, true},
		{`"\065\066"`, "AB", true},
		{`""`, "", true},
		{`unquoted`, "", false},
		{`"open`, "", false},
		{`"a" b`, "", false},
		{`"\256"`, "", false},
	}
	for _, c := range cases {
		got, ok := DecodeTXT(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("DecodeTXT(%s) = %q, %v; want %q, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestNormalize(t *testing.T) {
	ttl := 300
	r := Normalize(RRSet{Name: "WWW ", Type: "txt", TTL: &ttl, Values: []string{`"b"`, `"a" "c"`, `bare`}})
	if r.Name != "www" || r.Type != "TXT" || *r.TTL != 300 || !slices.Equal(r.Values, []string{"ac", "b", "bare"}) {
		t.Errorf("Normalize = %+v", r)
	}
	ttl = 1
	if *r.TTL != 300 {
		t.Error("TTL aliases the input")
	}
	if Normalize(RRSet{Type: "A"}).Name != Apex {
		t.Error("empty name is not the apex")
	}
	a := Normalize(RRSet{Name: "x", Type: "A", Values: []string{"192.0.2.2", "192.0.2.1"}})
	b := Normalize(RRSet{Name: "x", Type: "A", Values: []string{"192.0.2.1", "192.0.2.2"}, Labels: map[string]string{"k": "v"}})
	if a.Hash() != b.Hash() {
		t.Error("hash depends on value order or labels")
	}
	c := Normalize(RRSet{Name: "x", Type: "A", TTL: &ttl, Values: []string{"192.0.2.1", "192.0.2.2"}})
	if a.Hash() == c.Hash() {
		t.Error("hash ignores the TTL")
	}
}

func TestResolveFromList(t *testing.T) {
	zones := []Zone{{ID: "1", Name: "example.com"}, {ID: "2", Name: "sub.example.com"}, {ID: "3", Name: "example.org"}}
	for fqdn, want := range map[string]string{
		"example.com": "1", "a.example.com": "1", "sub.example.com": "2", "x.sub.example.com.": "2", "A.EXAMPLE.ORG": "3",
	} {
		z, err := ResolveFromList(zones, fqdn)
		if err != nil || z.ID != want {
			t.Errorf("%s → %+v, %v; want zone %s", fqdn, z, err, want)
		}
	}
	if _, err := ResolveFromList(zones, "badexample.com"); !errors.Is(err, ErrZoneNotFound) {
		t.Errorf("badexample.com: %v", err)
	}
}

func TestFQDN(t *testing.T) {
	if FQDN("@", "Example.com.") != "example.com" || FQDN("WWW", "example.com") != "www.example.com" {
		t.Error("FQDN")
	}
}

// Review finding (#28): types the provider cannot manage are not importable or prunable (03 §3).
func TestUnmanagedTypesOnlyLeave(t *testing.T) {
	for _, typ := range []string{"SRV", "CAA"} {
		r := Normalize(RRSet{Name: "x", Type: typ, Values: []string{"v"}})
		if got := AllowedDecisions(r, caps); !slices.Equal(got, []Decision{Leave}) {
			t.Errorf("%s: allowed = %v", typ, got)
		}
		if err := CheckDecision(r, caps, Import); err == nil {
			t.Errorf("%s: import accepted", typ)
		}
	}
}
