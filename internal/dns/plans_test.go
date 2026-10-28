package dns_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/dns"
	"git.juzo.io/juzo/hashi/internal/dns/hetzner"
	"git.juzo.io/juzo/hashi/internal/dns/hetzner/hetznertest"
	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
	hsync "git.juzo.io/juzo/hashi/internal/sync"
	"git.juzo.io/juzo/hashi/internal/vault"
)

type writeFixture struct {
	svc  *dns.Service
	eng  *hsync.Engine
	sim  *hetznertest.Sim
	url  string // the fake's adapter base URL
	zone string
}

func newWriteFixture(t *testing.T) writeFixture {
	t.Helper()
	st := storetest.New(t)
	v, err := vault.New(bytes.Repeat([]byte{7}, vault.KeySize), 1)
	if err != nil {
		t.Fatal(err)
	}
	sim, srv := hetznertest.NewSimServer(t)
	open := func(_ string, token secrets.Value) (dns.Provider, error) {
		return hetzner.New(token, hetzner.WithBaseURL(srv.URL+"/v1"), hetzner.WithClock(timeNow, noSleep)), nil
	}
	a := audit.New([]byte("k"))
	svc := dns.NewService(st, v, a, open)
	res, err := svc.Connect(t.Context(), "test", "", dns.ConnectInput{Type: "hetzner", Token: secrets.New(hetznertest.Token),
		DefaultTTL: 3600, RootDomain: "example.com"})
	if err != nil {
		t.Fatal(err)
	}
	eng := hsync.New(st, a, svc.Subsystems()...)
	return writeFixture{svc: svc, eng: eng, sim: sim, url: srv.URL + "/v1", zone: res.Zone.ID}
}

func (f writeFixture) auto(t *testing.T, sub string) *hsync.Plan {
	t.Helper()
	p, err := f.eng.Auto(t.Context(), sub, f.zone, hsync.TriggerSave, "admin")
	if err != nil {
		t.Fatalf("%s: %v", sub, err)
	}
	return p
}

func (f writeFixture) approveApply(t *testing.T, p *hsync.Plan) *hsync.Plan {
	t.Helper()
	if err := f.eng.Approve(t.Context(), p.ID, "admin", ""); err != nil {
		t.Fatal(err)
	}
	got, err := f.eng.Apply(t.Context(), p.ID, "admin", "")
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func ttl(n int) *int { return &n }

// A manual entry is created at the provider with the owner label, verified, and then plans nothing; a change applies
// automatically; deleting it is high risk and waits for approval.
func TestManualEntryLifecycle(t *testing.T) {
	f := newWriteFixture(t)
	ctx := t.Context()
	rec, err := f.svc.CreateRecord(ctx, "admin", "", f.zone, dns.RecordInput{Name: "_hashi-test.example.com", Type: "a",
		TTL: ttl(300), Values: []string{"192.0.2.10"}})
	if err != nil {
		t.Fatal(err)
	}
	if rec.RRSet.Name != "_hashi-test" || rec.Origin != dns.OriginUser {
		t.Fatalf("record %+v", rec)
	}
	p := f.auto(t, dns.SubsystemRecords)
	if p.Status != hsync.StatusApplied || len(p.Items) != 1 || p.Items[0].Action != "create" || !strings.Contains(p.Result, "verified") {
		t.Fatalf("plan %+v", p)
	}
	vals, gotTTL, labels, ok := f.sim.RRSet("100001", "_hashi-test", "A")
	inst, _ := f.svc.InstanceID(ctx)
	if !ok || !slices.Equal(vals, []string{"192.0.2.10"}) || *gotTTL != 300 || labels[hetzner.OwnerLabel] != inst {
		t.Fatalf("provider has %v ttl %v labels %v", vals, gotTTL, labels)
	}
	if p := f.auto(t, dns.SubsystemRecords); p != nil {
		t.Fatalf("second plan: %+v", p.Items)
	}

	if _, err := f.svc.UpdateRecord(ctx, "admin", "", rec.ID, dns.RecordInput{Values: []string{"192.0.2.11", "192.0.2.12"}}); err != nil {
		t.Fatal(err)
	}
	if p := f.auto(t, dns.SubsystemRecords); p.Status != hsync.StatusApplied || p.Items[0].Action != "update" {
		t.Fatalf("update plan %+v", p)
	}
	if vals, gotTTL, _, _ := f.sim.RRSet("100001", "_hashi-test", "A"); len(vals) != 2 || gotTTL != nil {
		t.Fatalf("after update %v ttl %v", vals, gotTTL)
	}

	if _, err := f.svc.DeleteRecord(ctx, "admin", "", rec.ID); err != nil {
		t.Fatal(err)
	}
	p = f.auto(t, dns.SubsystemRecords)
	if p.Status != hsync.StatusPending || p.Risk != hsync.High {
		t.Fatalf("delete plan %+v", p)
	}
	if _, _, _, ok := f.sim.RRSet("100001", "_hashi-test", "A"); !ok {
		t.Fatal("deleted before approval")
	}
	if got := f.approveApply(t, p); got.Status != hsync.StatusApplied {
		t.Fatalf("applied %+v", got)
	}
	if _, _, _, ok := f.sim.RRSet("100001", "_hashi-test", "A"); ok {
		t.Fatal("still at the provider")
	}
	if p := f.auto(t, dns.SubsystemRecords); p != nil {
		t.Fatalf("plan after delete: %+v", p.Items)
	}
}

// TXT entries are stored as plain text and sent quoted and split.
func TestManualTXTEntry(t *testing.T) {
	f := newWriteFixture(t)
	long := strings.Repeat("k", 300)
	if _, err := f.svc.CreateRecord(t.Context(), "admin", "", f.zone, dns.RecordInput{Name: "_hashi-test", Type: "TXT",
		Values: []string{`v=1 "q"`, long}}); err != nil {
		t.Fatal(err)
	}
	if p := f.auto(t, dns.SubsystemRecords); p.Status != hsync.StatusApplied {
		t.Fatalf("plan %+v", p)
	}
	vals, _, _, _ := f.sim.RRSet("100001", "_hashi-test", "TXT")
	if !slices.Contains(vals, `"v=1 \"q\""`) || !slices.Contains(vals, `"`+long[:255]+`" "`+long[255:]+`"`) {
		t.Errorf("stored %q", vals)
	}
	if p := f.auto(t, dns.SubsystemRecords); p != nil {
		t.Fatalf("TXT not idempotent: %+v", p.Items)
	}
}

func TestEntryValidation(t *testing.T) {
	f := newWriteFixture(t)
	for _, in := range []dns.RecordInput{
		{Name: "@", Type: "NS", Values: []string{"ns.example.net"}},
		{Name: "@", Type: "SOA", Values: []string{"x"}},
		{Name: "x", Type: "A", Values: []string{"2001:db8::1"}},
		{Name: "x", Type: "AAAA", Values: []string{"192.0.2.1"}},
		{Name: "@", Type: "CNAME", Values: []string{"example.net"}},
		{Name: "x", Type: "CNAME", Values: []string{"a.example.net", "b.example.net"}},
		{Name: "x", Type: "MX", Values: []string{"mail.example.net"}},
		{Name: "x", Type: "SRV", Values: []string{"10 5 443 t.example.net"}},
		{Name: "_sip._tcp", Type: "SRV", Values: []string{"10 5 t.example.net"}},
		{Name: "x", Type: "CAA", Values: []string{`0 policy "x"`}},
		{Name: "x", Type: "A", TTL: ttl(10), Values: []string{"192.0.2.1"}},
		{Name: "bad name", Type: "A", Values: []string{"192.0.2.1"}},
		{Name: "x", Type: "HINFO", Values: []string{"a b"}},
	} {
		if _, err := f.svc.CreateRecord(t.Context(), "admin", "", f.zone, in); !errors.Is(err, dns.ErrInvalid) {
			t.Errorf("%+v: %v", in, err)
		}
	}
	rec, err := f.svc.CreateRecord(t.Context(), "admin", "", f.zone, dns.RecordInput{Name: "_sip._tcp._hashi-test", Type: "srv",
		Values: []string{"10 5 5060 Sip.Example.net"}})
	if err != nil || rec.RRSet.Values[0] != "10 5 5060 sip.example.net." {
		t.Errorf("SRV %+v %v", rec, err)
	}
}

// Invariant 1: an RRSet Hashi does not own is never changed. Creating an entry over it is refused, and an entry whose
// RRSet appears at the provider before Hashi created it becomes a conflict, never an overwrite.
func TestInvariant_1_UnownedRRSetsUntouched(t *testing.T) {
	f := newWriteFixture(t)
	ctx := t.Context()
	if _, err := f.svc.CreateRecord(ctx, "admin", "", f.zone, dns.RecordInput{Name: "svc1", Type: "A", Values: []string{"192.0.2.99"}}); !errors.Is(err, dns.ErrConflict) {
		t.Fatalf("entry over an unowned RRSet: %v", err)
	}
	if _, err := f.svc.CreateRecord(ctx, "admin", "", f.zone, dns.RecordInput{Name: "svc2", Type: "A", Values: []string{"192.0.2.99"}}); !errors.Is(err, dns.ErrConflict) {
		t.Fatalf("A beside an unowned CNAME: %v", err)
	}

	// Someone creates the same RRSet in the console between saving and planning.
	if _, err := f.svc.CreateRecord(ctx, "admin", "", f.zone, dns.RecordInput{Name: "race._hashi-test", Type: "A", Values: []string{"192.0.2.10"}}); err != nil {
		t.Fatal(err)
	}
	console := hetzner.New(secrets.New(hetznertest.Token), hetzner.WithBaseURL(f.url), hetzner.WithClock(timeNow, noSleep))
	other := dns.Normalize(dns.RRSet{Name: "race._hashi-test", Type: "A", Values: []string{"198.51.100.7"}})
	if err := console.Apply(ctx, dns.Zone{ID: "100001"}, dns.ProviderPlan{Changes: []dns.Change{{Op: dns.OpCreate, After: &other}}}); err != nil {
		t.Fatal(err)
	}
	writes := len(f.sim.Writes)
	if p := f.auto(t, dns.SubsystemRecords); p != nil {
		t.Fatalf("plan over a foreign RRSet: %+v", p.Items)
	}
	d, err := f.svc.Subsystems()[0].Plan(ctx, f.zone)
	if err != nil || len(d.Items) != 1 || d.Items[0].Action != hsync.ActionConflict {
		t.Fatalf("draft %+v %v", d, err)
	}
	if vals, _, _, _ := f.sim.RRSet("100001", "race._hashi-test", "A"); vals[0] != "198.51.100.7" || len(f.sim.Writes) != writes {
		t.Fatalf("foreign RRSet changed: %v, writes %v", vals, f.sim.Writes[writes:])
	}
}

// Invariant 2: no plan of any DNS subsystem touches NS or SOA, whatever is decided or saved.
func TestInvariant_2_NoPlanTouchesNSSOA(t *testing.T) {
	f := newWriteFixture(t)
	ctx := t.Context()
	if _, err := f.svc.CreateRecord(ctx, "admin", "", f.zone, dns.RecordInput{Name: "_hashi-test", Type: "A", Values: []string{"192.0.2.1"}}); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.SaveDecisions(ctx, "admin", "", f.zone, []dns.DecisionInput{{Name: "svc2", Type: "CNAME", Decision: dns.Prune},
		{Name: "svc1", Type: "A", Decision: dns.Import}}); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.SaveDecisions(ctx, "admin", "", f.zone, []dns.DecisionInput{{Name: "@", Type: "NS", Decision: dns.Prune}}); err == nil {
		t.Fatal("NS prune accepted")
	}
	for _, sub := range f.svc.Subsystems() {
		d, err := sub.Plan(ctx, f.zone)
		if err != nil {
			t.Fatal(err)
		}
		for _, it := range d.Items {
			if strings.HasSuffix(it.Object, " NS") || strings.HasSuffix(it.Object, " SOA") {
				t.Errorf("%s plans %s %s", sub.Name(), it.Action, it.Object)
			}
		}
	}
	f.auto(t, dns.SubsystemRecords)
	f.approveApply(t, f.auto(t, dns.SubsystemPrune))
	f.approveApply(t, f.auto(t, dns.SubsystemProbe))
	for _, w := range f.sim.Writes {
		if strings.HasSuffix(w, "/NS") || strings.HasSuffix(w, "/SOA") || strings.Contains(w, "/NS/") || strings.Contains(w, "/SOA/") {
			t.Errorf("write to NS/SOA: %s", w)
		}
	}
}

// Invariant 3: when reading the zone fails, nothing is written.
func TestInvariant_3_FailedReadMeansNoWrites(t *testing.T) {
	f := newWriteFixture(t)
	ctx := t.Context()
	if _, err := f.svc.CreateRecord(ctx, "admin", "", f.zone, dns.RecordInput{Name: "_hashi-test", Type: "A", Values: []string{"192.0.2.1"}}); err != nil {
		t.Fatal(err)
	}
	f.sim.Fail = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/rrsets") {
			w.WriteHeader(http.StatusServiceUnavailable)
			return true
		}
		return false
	}
	if _, err := f.eng.Auto(ctx, dns.SubsystemRecords, f.zone, hsync.TriggerSave, "admin"); !errors.Is(err, dns.ErrValidation) {
		t.Fatalf("err = %v", err)
	}
	if len(f.sim.Writes) != 0 {
		t.Fatalf("writes after a failed read: %v", f.sim.Writes)
	}
}

// Invariant 4: the write test runs only after an admin approved its plan, then leaves nothing behind.
func TestInvariant_4_ProbeOnlyAfterApproval(t *testing.T) {
	f := newWriteFixture(t)
	p := f.auto(t, dns.SubsystemProbe)
	if p.Status != hsync.StatusPending || p.Risk != hsync.High || len(p.Items) != 2 {
		t.Fatalf("probe plan %+v", p)
	}
	if len(f.sim.Writes) != 0 {
		t.Fatalf("writes before approval: %v", f.sim.Writes)
	}
	got := f.approveApply(t, p)
	if got.Status != hsync.StatusApplied || !strings.Contains(got.Result, "write access confirmed") {
		t.Fatalf("probe %+v", got)
	}
	if len(f.sim.Writes) != 2 || !strings.HasPrefix(f.sim.Writes[0], "POST ") || !strings.HasPrefix(f.sim.Writes[1], "DELETE ") {
		t.Fatalf("writes %v", f.sim.Writes)
	}
	if _, _, _, ok := f.sim.RRSet("100001", dns.ProbeName, "TXT"); ok {
		t.Fatal("probe left behind")
	}
}

// Import keeps the RRSet as it is (only the owner label is added); prune deletes exactly the marked RRSets after approval.
func TestImportAndPrune(t *testing.T) {
	f := newWriteFixture(t)
	ctx := t.Context()
	before, _, _, _ := f.sim.RRSet("100001", "svc1", "A")
	if err := f.svc.SaveDecisions(ctx, "admin", "", f.zone, []dns.DecisionInput{{Name: "svc1", Type: "A", Decision: dns.Import},
		{Name: "svc2", Type: "CNAME", Decision: dns.Prune}}); err != nil {
		t.Fatal(err)
	}
	p := f.auto(t, dns.SubsystemRecords)
	if p.Status != hsync.StatusApplied || len(p.Items) != 1 || p.Items[0].Action != "label" {
		t.Fatalf("import plan %+v", p)
	}
	after, _, labels, _ := f.sim.RRSet("100001", "svc1", "A")
	if !slices.Equal(before, after) || labels[hetzner.OwnerLabel] == "" {
		t.Fatalf("import changed the RRSet: %v → %v %v", before, after, labels)
	}

	pr := f.auto(t, dns.SubsystemPrune)
	if pr.Status != hsync.StatusPending || len(pr.Items) != 1 || pr.Items[0].Object != "svc2 CNAME" {
		t.Fatalf("prune plan %+v", pr)
	}
	f.approveApply(t, pr)
	if _, _, _, ok := f.sim.RRSet("100001", "svc2", "CNAME"); ok {
		t.Fatal("svc2 still there")
	}
	if p := f.auto(t, dns.SubsystemPrune); p != nil {
		t.Fatalf("prune plan after prune: %+v", p.Items)
	}
}

func timeNow() time.Time { return time.Now() }

func noSleep(context.Context, time.Duration) error { return nil }
