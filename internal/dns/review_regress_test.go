package dns_test

import (
	"bytes"
	"net/http"
	"strings"
	"testing"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/dns"
	"git.juzo.io/juzo/hashi/internal/dns/hetzner"
	"git.juzo.io/juzo/hashi/internal/dns/hetzner/hetznertest"
	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/store/db"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
	hsync "git.juzo.io/juzo/hashi/internal/sync"
	"git.juzo.io/juzo/hashi/internal/vault"
)

// Review #30 R1: an import saved in S2 (ownership row only, no entry) must not turn into a delete; it gets its entry.
func TestImportFromS2KeepsTheRRSet(t *testing.T) {
	st := storetest.New(t)
	v, _ := vault.New(bytes.Repeat([]byte{7}, vault.KeySize), 1)
	_, srv := hetznertest.NewSimServer(t)
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
	if err := st.Q.UpsertDNSOwnership(t.Context(), db.UpsertDNSOwnershipParams{ZoneID: res.Zone.ID, Name: "svc1", Type: "A",
		Origin: dns.OriginImported, CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	eng := hsync.New(st, a, svc.Subsystems()...)
	for _, sub := range []string{dns.SubsystemRecords, dns.SubsystemRecordDeletes} {
		p, err := eng.Auto(t.Context(), sub, res.Zone.ID, hsync.TriggerReconcile, audit.System)
		if err != nil {
			t.Fatal(err)
		}
		if p != nil {
			for _, it := range p.Items {
				if it.Action != "label" {
					t.Errorf("%s: %s %s", sub, it.Action, it.Object)
				}
			}
		}
	}
	recs, _ := svc.Records(t.Context(), st.R, res.Zone.ID)
	if len(recs) != 1 || recs[0].RRSet.Name != "svc1" || recs[0].Origin != dns.OriginImported {
		t.Errorf("entries %+v", recs)
	}
}

// Review #30 R2 (invariant 16): a deletion waiting for approval does not hold back a safe change.
func TestPendingDeleteDoesNotBlockSafeChanges(t *testing.T) {
	f := newWriteFixture(t)
	ctx := t.Context()
	rec, err := f.svc.CreateRecord(ctx, "admin", "", f.zone, dns.RecordInput{Name: "a._hashi-test", Type: "A", Values: []string{"192.0.2.1"}})
	if err != nil {
		t.Fatal(err)
	}
	f.auto(t, dns.SubsystemRecords)
	if _, err := f.svc.DeleteRecord(ctx, "admin", "", rec.ID); err != nil {
		t.Fatal(err)
	}
	if p := f.auto(t, dns.SubsystemRecordDeletes); p == nil || p.Status != hsync.StatusPending {
		t.Fatalf("delete plan %+v", p)
	}
	if _, err := f.svc.CreateRecord(ctx, "admin", "", f.zone, dns.RecordInput{Name: "b._hashi-test", Type: "A", Values: []string{"192.0.2.2"}}); err != nil {
		t.Fatal(err)
	}
	if p := f.auto(t, dns.SubsystemRecords); p == nil || p.Status != hsync.StatusApplied {
		t.Fatalf("safe plan %+v", p)
	}
	if _, _, _, ok := f.sim.RRSet("100001", "b._hashi-test", "A"); !ok {
		t.Error("b not created while the delete waits")
	}
	if _, _, _, ok := f.sim.RRSet("100001", "a._hashi-test", "A"); !ok {
		t.Error("a deleted without approval")
	}
}

// Review #30 R3: after a partial failure, what Hashi did create stays Hashi's.
func TestPartialApplyKeepsWhatWasCreated(t *testing.T) {
	f := newWriteFixture(t)
	ctx := t.Context()
	for _, n := range []string{"a._hashi-test", "b._hashi-test"} {
		if _, err := f.svc.CreateRecord(ctx, "admin", "", f.zone, dns.RecordInput{Name: n, Type: "A", Values: []string{"192.0.2.1"}}); err != nil {
			t.Fatal(err)
		}
	}
	posts := 0
	f.sim.Fail = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/rrsets") {
			if posts++; posts == 2 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return true
			}
		}
		return false
	}
	if _, err := f.eng.Auto(ctx, dns.SubsystemRecords, f.zone, hsync.TriggerSave, "admin"); err == nil {
		t.Fatal("no error")
	}
	f.sim.Fail = nil
	p := f.auto(t, dns.SubsystemRecords)
	if p == nil || len(p.Items) != 1 || p.Items[0].Action != "create" || p.Items[0].Object != "b._hashi-test A" || p.Status != hsync.StatusApplied {
		t.Fatalf("retry plan %+v", p)
	}
}

// Review #30 R4: the write test removes its probe even when the create step fails after the provider accepted it.
func TestProbeCleansUpAfterFailedCreate(t *testing.T) {
	f := newWriteFixture(t)
	p := f.auto(t, dns.SubsystemProbe)
	polls := 0
	f.sim.Fail = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/actions/") {
			if polls++; polls <= 3 { // the create's Action polls fail (and their retries)
				w.WriteHeader(http.StatusServiceUnavailable)
				return true
			}
		}
		return false
	}
	_ = f.eng.Approve(t.Context(), p.ID, "admin", "")
	if _, err := f.eng.Apply(t.Context(), p.ID, "admin", ""); err == nil {
		t.Fatal("no error")
	}
	f.sim.Fail = nil
	if _, _, _, ok := f.sim.RRSet("100001", dns.ProbeName, "TXT"); ok {
		t.Error("probe TXT left at the provider")
	}
}

// 03 §10: CNAME exclusivity in the planner: a CNAME entry beside an RRSet that stays is a conflict, not a create.
func TestPlannerCNAMEExclusivity(t *testing.T) {
	f := newWriteFixture(t)
	ctx := t.Context()
	if _, err := f.svc.CreateRecord(ctx, "admin", "", f.zone, dns.RecordInput{Name: "c._hashi-test", Type: "CNAME", Values: []string{"example.net"}}); err != nil {
		t.Fatal(err)
	}
	console := hetzner.New(secrets.New(hetznertest.Token), hetzner.WithBaseURL(f.url), hetzner.WithClock(timeNow, noSleep))
	other := dns.Normalize(dns.RRSet{Name: "c._hashi-test", Type: "TXT", Values: []string{"x"}})
	if err := console.Apply(ctx, dns.Zone{ID: "100001"}, dns.ProviderPlan{Changes: []dns.Change{{Op: dns.OpCreate, After: &other}}}); err != nil {
		t.Fatal(err)
	}
	d, err := f.svc.Subsystems()[0].Plan(ctx, f.zone)
	if err != nil || len(d.Items) != 1 || d.Items[0].Action != hsync.ActionConflict || !strings.Contains(d.Items[0].Note, "CNAME") {
		t.Fatalf("draft %+v %v", d.Items, err)
	}
}
