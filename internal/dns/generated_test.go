package dns_test

import (
	"context"
	"net/netip"
	"slices"
	"strings"
	"testing"

	"git.juzo.io/juzo/hashi/internal/dns"
	"git.juzo.io/juzo/hashi/internal/ipsets"
	hsync "git.juzo.io/juzo/hashi/internal/sync"
)

// gen is a generator the test changes between plans.
type gen struct{ recs []dns.Generated }

func (g *gen) fn(context.Context, string) ([]dns.Generated, error) { return g.recs, nil }

func cname(name, target, src string) dns.Generated {
	return dns.Generated{Name: name + ".example.com", Type: "CNAME", Values: []string{target + ".example.com."}, Source: src}
}

func items(p *hsync.Plan) []string {
	if p == nil {
		return nil
	}
	var out []string
	for _, it := range p.Items {
		out = append(out, it.Action+" "+it.Object)
	}
	return out
}

// Generated records (03 §5.1–5.2) go through the normal plan: an existing RRSet that is not Hashi's is a conflict;
// an imported entry at a generated name is taken over (a migrated service record) and its entry row goes; another
// type at that name is deleted first (high risk) and the CNAME follows; a manual entry wins; a record no longer
// generated is deleted after approval.
func TestGeneratedRecords(t *testing.T) {
	f := newWriteFixture(t)
	ctx := t.Context()
	g := &gen{recs: []dns.Generated{
		{Name: "edge-a.example.com", Type: "A", Values: []string{"192.0.2.1"}, Source: "host edge-a"},
		cname("via.edge-a", "edge-a", "host edge-a"), cname("on.edge-a", "via.edge-a", "host edge-a"),
		cname("svc2", "on.edge-a", "service Two"),
	}}
	f.svc.UseGenerator(g.fn)

	// Conflicts only: no plan (the engine stores none), no writes.
	if p := f.auto(t, dns.SubsystemRecords); p != nil {
		t.Fatalf("unowned generated names must be conflicts only: %v", items(p))
	}
	if len(f.sim.Writes) != 0 {
		t.Fatalf("writes before import: %v", f.sim.Writes)
	}

	// The admin imports the old records; the generated ones take them over without changing their values.
	var dec []dns.DecisionInput
	for _, k := range [][2]string{{"edge-a", "A"}, {"via.edge-a", "CNAME"}, {"on.edge-a", "CNAME"}, {"svc2", "CNAME"}, {"svc1", "A"}} {
		dec = append(dec, dns.DecisionInput{Name: k[0], Type: k[1], Decision: dns.Import})
	}
	if err := f.svc.SaveDecisions(ctx, "admin", "", f.zone, dec); err != nil {
		t.Fatal(err)
	}
	g.recs = append(g.recs, cname("new", "on.edge-a", "service New"), cname("svc1", "on.edge-a", "service One"))
	p := f.auto(t, dns.SubsystemRecords)
	if p.Status != hsync.StatusApplied {
		t.Fatalf("records plan %s %v", p.Status, items(p))
	}
	got := items(p)
	if !slices.Contains(got, "create new CNAME") || !slices.Contains(got, "label svc2 CNAME") ||
		!slices.Contains(got, "conflict svc1 CNAME") {
		t.Fatalf("records plan items %v", got)
	}
	if vals, _, _, ok := f.sim.RRSet("100001", "new", "CNAME"); !ok || vals[0] != "on.edge-a.example.com." {
		t.Fatalf("new CNAME %v %v", vals, ok)
	}
	recs, err := f.svc.Records(ctx, f.st.R, f.zone)
	if err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(recs, func(r dns.Record) bool { return r.RRSet.Name == "svc2" }) {
		t.Fatal("the imported svc2 entry should be taken over by the generated record")
	}
	obs, _ := f.svc.RRSets(ctx, f.zone, "", "svc2")
	if i := slices.IndexFunc(obs, func(o dns.Observed) bool { return o.Name == "svc2" }); i < 0 || obs[i].Origin != dns.OriginManaged {
		t.Fatalf("svc2 is not managed: %+v", obs)
	}

	// svc1 A (imported) must go before svc1 CNAME can exist: a high-risk delete.
	d := f.auto(t, dns.SubsystemRecordDeletes)
	if got := items(d); d.Status != hsync.StatusPending || len(got) != 1 || got[0] != "delete svc1 A" {
		t.Fatalf("delete plan %s %v", d.Status, got)
	}
	f.approveApply(t, d)
	p = f.auto(t, dns.SubsystemRecords)
	if got := items(p); p.Status != hsync.StatusApplied || !slices.Contains(got, "create svc1 CNAME") {
		t.Fatalf("after the delete %v", got)
	}

	// A manual entry at a generated name wins.
	if _, err := f.svc.CreateRecord(ctx, "admin", "", f.zone, dns.RecordInput{Name: "manual", Type: "A", Values: []string{"192.0.2.50"}}); err != nil {
		t.Fatal(err)
	}
	g.recs = append(g.recs, cname("manual", "on.edge-a", "service Manual"))
	p = f.auto(t, dns.SubsystemRecords)
	if got := items(p); !slices.Contains(got, "create manual A") || !slices.Contains(got, "conflict manual CNAME") {
		t.Fatalf("manual entry plan %v", got)
	}

	// "new" is no longer generated: deleted after approval, ownership gone.
	g.recs = slices.DeleteFunc(g.recs, func(r dns.Generated) bool { return strings.HasPrefix(r.Name, "new.") })
	d = f.auto(t, dns.SubsystemRecordDeletes)
	if got := items(d); d.Status != hsync.StatusPending || len(got) != 1 || got[0] != "delete new CNAME" {
		t.Fatalf("delete of a dropped record %v", got)
	}
	f.approveApply(t, d)
	if _, _, _, ok := f.sim.RRSet("100001", "new", "CNAME"); ok {
		t.Fatal("new CNAME still there")
	}
	if p := f.auto(t, dns.SubsystemRecordDeletes); p != nil {
		t.Fatalf("another delete plan %v", items(p))
	}
}

// Invariant 21 for DNS values: an A entry whose address set resolves to nothing keeps the values the provider holds;
// it is neither emptied nor deleted. One that never resolved is simply not created.
func TestInvariant_21_DNSValueSetKeepsLastApplied(t *testing.T) {
	f := newWriteFixture(t)
	ctx := t.Context()
	var addrs []netip.Addr
	f.svc.UseTargets(func(context.Context, string, string) (ipsets.Target, error) {
		if len(addrs) == 0 {
			return ipsets.Target{Unresolved: "agent never seen"}, nil
		}
		return ipsets.Target{Addresses: addrs}, nil
	})
	if _, err := f.svc.CreateRecord(ctx, "admin", "", f.zone, dns.RecordInput{Name: "home", Type: "A", ValueSetID: "set"}); err == nil {
		t.Fatal("a missing set must be refused")
	}
	set := addressSet(t, f)
	if _, err := f.svc.CreateRecord(ctx, "admin", "", f.zone, dns.RecordInput{Name: "home", Type: "A", ValueSetID: set}); err != nil {
		t.Fatal(err)
	}
	if p := f.auto(t, dns.SubsystemRecords); p != nil {
		t.Fatalf("never resolved: nothing to create, got %v", items(p))
	}
	addrs = []netip.Addr{netip.MustParseAddr("192.0.2.7"), netip.MustParseAddr("2001:db8::7"), netip.MustParseAddr("192.0.2.8")}
	p := f.auto(t, dns.SubsystemRecords)
	if p == nil || p.Status != hsync.StatusApplied {
		t.Fatalf("resolved plan %v", items(p))
	}
	if vals, _, _, _ := f.sim.RRSet("100001", "home", "A"); !slices.Equal(vals, []string{"192.0.2.7", "192.0.2.8"}) {
		t.Fatalf("A values %v (IPv6 must be left out)", vals)
	}
	addrs = nil
	for _, sub := range dns.EntrySubsystems {
		if p := f.auto(t, sub); p != nil {
			t.Fatalf("%s after the set emptied: %v", sub, items(p))
		}
	}
	if vals, _, _, ok := f.sim.RRSet("100001", "home", "A"); !ok || len(vals) != 2 {
		t.Fatalf("kept values %v %v", vals, ok)
	}
}

// addressSet creates a user set with one ip item.
func addressSet(t *testing.T, f writeFixture) string {
	t.Helper()
	s, err := ipsets.CreateSet(t.Context(), f.st.Q, ipsets.SetInput{Slug: "home", Name: "Home"}, timeNow())
	if err != nil {
		t.Fatal(err)
	}
	it, _ := ipsets.ParseIPOrCIDR("192.0.2.7")
	if _, err := (ipsets.Editor{}).AddElement(t.Context(), f.st.Q, s.ID, ipsets.ElementInput{Item: it}, "t", timeNow()); err != nil {
		t.Fatal(err)
	}
	return s.ID
}

// Review of PR C: a generated record takes over only an imported entry of its own type (and, as a CNAME, every type
// at its name); the apex MX and TXT next to a flattened apex A stay.
func TestGeneratedRecordKeepsOtherImportedTypes(t *testing.T) {
	f := newWriteFixture(t)
	ctx := t.Context()
	if err := f.svc.SaveDecisions(ctx, "admin", "", f.zone, []dns.DecisionInput{{Name: "@", Type: "A", Decision: dns.Import},
		{Name: "@", Type: "MX", Decision: dns.Import}, {Name: "@", Type: "TXT", Decision: dns.Import}}); err != nil {
		t.Fatal(err)
	}
	f.svc.UseGenerator((&gen{recs: []dns.Generated{{Name: "example.com", Type: "A", Values: []string{"192.0.2.1"}, Source: "service Root"}}}).fn)
	f.auto(t, dns.SubsystemRecords)
	if p := f.auto(t, dns.SubsystemRecordDeletes); p != nil {
		t.Fatalf("other imported types at a generated name must stay: %v", items(p))
	}
}

// Review of PR C, 03 §5.1: the ovr record no service uses any more is deleted in the same plan as the service record
// change (a routing indirection Hashi generated: low risk); it is not left to a separate delete plan.
func TestUnusedOverrideRecordGoesWithTheServiceChange(t *testing.T) {
	f := newWriteFixture(t)
	const ovr = "edge-a.ovr.on.edge-b"
	g := &gen{recs: []dns.Generated{cname(ovr, "via.edge-a", "service X"), cname("x-new", ovr, "service X")}}
	f.svc.UseGenerator(g.fn)
	if p := f.auto(t, dns.SubsystemRecords); p == nil || p.Status != hsync.StatusApplied {
		t.Fatalf("first plan %v", items(p))
	}
	g.recs = []dns.Generated{cname("x-new", "on.edge-b", "service X")}
	p := f.auto(t, dns.SubsystemRecords)
	got := items(p)
	if p.Status != hsync.StatusApplied || !slices.Contains(got, "update x-new CNAME") || !slices.Contains(got, "delete "+ovr+" CNAME") {
		t.Fatalf("override drop plan %s %v", p.Status, got)
	}
	if d := f.auto(t, dns.SubsystemRecordDeletes); d != nil {
		t.Fatalf("separate delete plan %v", items(d))
	}
	if _, _, _, ok := f.sim.RRSet("100001", ovr, "CNAME"); ok {
		t.Fatal("ovr record still there")
	}
}
