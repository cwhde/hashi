package firewall

import (
	"database/sql"
	"net/netip"
	"slices"
	"strings"
	"testing"

	"git.juzo.io/juzo/hashi/internal/dns"
	"git.juzo.io/juzo/hashi/internal/services"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

func host(id, name, pub string, subnets ...string) Effective {
	e := Effective{Row: db.FirewallHost{ID: id, Name: name}}
	if pub != "" {
		e.PublicIP = netip.MustParseAddr(pub)
	}
	for _, s := range subnets {
		e.Subnets = append(e.Subnets, netip.MustParsePrefix(s))
	}
	return e
}

func via(h Effective, to string) Effective {
	h.Row.RouteViaHostID = sql.NullString{String: to, Valid: true}
	return h
}

func svc(name, hostName, addr string) services.Service {
	return services.Service{ID: name, Name: name, Slug: name, Enabled: true, HostName: hostName,
		Target: services.Target{Address: addr, Port: 80}}
}

func lines(gs []dns.Generated) []string {
	var out []string
	for _, g := range gs {
		out = append(out, g.Name+" "+g.Type+" "+strings.Join(g.Values, ","))
	}
	slices.Sort(out)
	return out
}

func has(t *testing.T, got []string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !slices.Contains(got, w) {
			t.Errorf("missing %q in\n%s", w, strings.Join(got, "\n"))
		}
	}
}

// Host records and the host routing override (03 §5.1); service records: CNAME to on.<home> when a target lies
// behind a host (subnet, host address, host FQDN), A with public addresses otherwise, nothing for a private address
// no host covers (03 §5.2); apex flattened to the entry host's address.
func TestGenerateHostAndServiceRecords(t *testing.T) {
	a := host("a", "edge-a", "192.0.2.1", "10.0.4.0/24")
	b := via(host("b", "edge-b", "198.51.100.3", "10.0.0.0/22"), "a")
	noPub := host("c", "edge-c", "", "10.9.0.0/24")
	got := lines(GenerateRecords("Example.com.", []Effective{a, b, noPub}, []services.Service{
		svc("wiki", "wiki.example.com", "10.0.4.20"),
		svc("nas", "nas.example.com", "10.0.1.5"),
		svc("byname", "byname.example.com", "edge-a.example.com"),
		svc("public", "public.example.com", "203.0.113.9"),
		svc("lan", "lan.example.com", "192.168.77.1"),
		svc("apex", "example.com", "10.0.0.9"),
		svc("hidden", "hidden.example.com", "10.9.0.4"),
		svc("clash", "edge-a.example.com", "10.0.4.9"),
	}))
	has(t, got,
		"edge-a.example.com. A 192.0.2.1", "via.edge-a.example.com. CNAME edge-a.example.com.",
		"on.edge-a.example.com. CNAME via.edge-a.example.com.",
		"edge-b.example.com. A 198.51.100.3", "via.edge-b.example.com. CNAME edge-b.example.com.",
		"on.edge-b.example.com. CNAME via.edge-a.example.com.",
		"wiki.example.com. CNAME on.edge-a.example.com.", "nas.example.com. CNAME on.edge-b.example.com.",
		"byname.example.com. CNAME on.edge-a.example.com.", "public.example.com. A 203.0.113.9",
		"example.com. A 192.0.2.1")
	for _, g := range got {
		if strings.HasPrefix(g, "lan.") || strings.Contains(g, "edge-c") || strings.HasPrefix(g, "hidden.") ||
			g == "edge-a.example.com. CNAME on.edge-a.example.com." {
			t.Errorf("unexpected %q", g)
		}
	}
	if len(got) != 11 {
		t.Errorf("%d records:\n%s", len(got), strings.Join(got, "\n"))
	}
}

// Service routing override (03 §5.1, §10): one shared ovr record per (home, entry) pair; the record exists while one
// service needs it; an entry host equal to where on.<home> already points gives the plain on. CNAME; changing the
// host override later moves services between the two forms.
func TestServiceRoutingOverride(t *testing.T) {
	a := host("a", "edge-a", "192.0.2.1", "10.0.4.0/24")
	b := host("b", "edge-b", "198.51.100.3", "10.0.0.0/22")
	one, two := svc("one", "one.example.com", "10.0.1.1"), svc("two", "two.example.com", "10.0.1.2")
	one.EntryHostID, two.EntryHostID = "a", "a"
	const ovr = "edge-a.ovr.on.edge-b.example.com. CNAME via.edge-a.example.com."
	count := func(got []string, s string) int {
		n := 0
		for _, g := range got {
			if g == s {
				n++
			}
		}
		return n
	}

	got := lines(GenerateRecords("example.com", []Effective{a, b}, []services.Service{one, two}))
	has(t, got, ovr, "one.example.com. CNAME edge-a.ovr.on.edge-b.example.com.", "two.example.com. CNAME edge-a.ovr.on.edge-b.example.com.")
	if count(got, ovr) != 2 { // two services, one name: the DNS plan dedups by name and type
		t.Fatalf("ovr entries %d", count(got, ovr))
	}

	two.EntryHostID = ""
	got = lines(GenerateRecords("example.com", []Effective{a, b}, []services.Service{one, two}))
	has(t, got, ovr, "two.example.com. CNAME on.edge-b.example.com.")

	one.Enabled = false
	got = lines(GenerateRecords("example.com", []Effective{a, b}, []services.Service{one, two}))
	if count(got, ovr) != 0 {
		t.Fatal("ovr record kept after its last user")
	}

	one.Enabled = true // on.edge-b now points at edge-a itself: plain on. CNAME, no ovr
	got = lines(GenerateRecords("example.com", []Effective{a, via(b, "a")}, []services.Service{one, two}))
	has(t, got, "one.example.com. CNAME on.edge-b.example.com.")
	if count(got, ovr) != 0 {
		t.Fatal("ovr generated although on.edge-b already enters through edge-a")
	}
}

// Generated names stay clear of each other: ovr, on and via are reserved host names, and generated record values
// are IPv4 only (03 §4).
func TestGeneratedRecordsIPv4Only(t *testing.T) {
	a := host("a", "edge-a", "192.0.2.1")
	a.PublicIP6 = netip.MustParseAddr("2001:db8::1")
	for _, g := range GenerateRecords("example.com", []Effective{a}, []services.Service{svc("v6", "v6.example.com", "2001:db8::99")}) {
		if g.Type == "AAAA" || strings.HasPrefix(g.Name, "v6.") {
			t.Errorf("unexpected %+v", g)
		}
	}
	for _, n := range ReservedNames {
		if !slices.Contains([]string{"on", "via", "ovr"}, n) {
			t.Errorf("reserved %q", n)
		}
	}
}

// Review of PR C: a host override to a host without a public IPv4 address would leave on.<host> dangling; the host
// keeps entering through itself until the override target has an address.
func TestHostOverrideToUnpublishedHost(t *testing.T) {
	a := via(host("a", "edge-a", "192.0.2.1", "10.0.4.0/24"), "c")
	c := host("c", "edge-c", "")
	got := lines(GenerateRecords("example.com", []Effective{a, c}, []services.Service{svc("wiki", "wiki.example.com", "10.0.4.20")}))
	has(t, got, "on.edge-a.example.com. CNAME via.edge-a.example.com.", "wiki.example.com. CNAME on.edge-a.example.com.")
}
