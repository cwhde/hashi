package hetzner

import (
	"encoding/json"
	"slices"
	"testing"

	"git.juzo.io/juzo/hashi/internal/dns/hetzner/hetznertest"
	"git.juzo.io/juzo/hashi/internal/secrets"
)

// The probe capture's RRSets of every managed type read back through the adapter's mapping.
func TestContract_ProbeRRSetsOfEveryManagedType(t *testing.T) {
	f := hetznertest.Load(t, "write/get_after_creates.json")
	var body struct {
		RRSets []rrsetJSON `json:"rrsets"`
	}
	if err := json.Unmarshal(f.Body, &body); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"write/get_sip_srv.json", "write/get_hashi-test_caa.json"} {
		var one struct {
			RRSet rrsetJSON `json:"rrset"`
		}
		if err := json.Unmarshal(hetznertest.Load(t, name).Body, &one); err != nil {
			t.Fatal(err)
		}
		body.RRSets = append(body.RRSets, one.RRSet)
	}
	got := map[string][]string{}
	for _, r := range body.RRSets {
		m := r.model()
		got[m.Name+"/"+m.Type] = m.Values
	}
	want := map[string][]string{
		"_hashi-test/A":    {"192.0.2.10", "192.0.2.11"},
		"_hashi-test/AAAA": {"2001:db8::10"},
		"_hashi-test/MX":   {"10 mx1._hashi-test.example.com.", "20 mx2._hashi-test.example.com."},
		// TXT: escaped quotes resolved, chunks joined.
		"_hashi-test/TXT": {"hashi contract probe", "part one part two", `with "escaped" quotes`},
		// CAA keeps its quoted value (only TXT is decoded).
		"_hashi-test/CAA":           {`0 issue "letsencrypt.org"`, `0 issuewild ";"`, `128 iodef "mailto:hostmaster@example.com"`},
		"_sip._tcp._hashi-test/SRV": {"10 60 5060 sip._hashi-test.example.com.", "20 0 5061 sip2._hashi-test.example.com."},
	}
	for k, v := range want {
		if !slices.Equal(got[k], v) {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
	caps, _ := New(secrets.New(hetznertest.Token)).Capabilities(t.Context())
	for _, typ := range []string{"A", "AAAA", "CNAME", "MX", "TXT", "SRV", "CAA"} {
		if !slices.Contains(caps.ManagedTypes, typ) {
			t.Errorf("%s not managed", typ)
		}
	}
}
