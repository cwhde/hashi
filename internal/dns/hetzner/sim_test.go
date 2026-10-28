package hetzner

import (
	"context"
	"errors"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/dns"
	"git.juzo.io/juzo/hashi/internal/dns/hetzner/hetznertest"
	"git.juzo.io/juzo/hashi/internal/secrets"
)

// Plan → Apply → Verify against the stateful fake: create with owner label, update, delete; a second plan is empty.
func TestPlanApplyVerify_Sim(t *testing.T) {
	sim, srv := hetznertest.NewSimServer(t)
	c := New(secrets.New(hetznertest.Token), WithBaseURL(srv.URL+"/v1"), WithClock(time.Now, noSleep))
	ctx := t.Context()
	zone := dns.Zone{ID: "100001", Name: "example.com", TTL: 3600}
	ttl := 600
	txt := dns.Normalize(dns.RRSet{Name: "_hashi-test", Type: "TXT", Values: []string{`say "hi"`}, Owner: "inst-1"})
	a := dns.Normalize(dns.RRSet{Name: "_hashi-test", Type: "A", TTL: &ttl, Values: []string{"192.0.2.10"}, Owner: "inst-1"})
	desired := []dns.RRSet{txt, a}
	plan, err := c.Plan(ctx, zone, desired, nil)
	if err != nil || len(plan.Changes) != 2 {
		t.Fatalf("plan %v %v", plan, err)
	}
	if err := c.Apply(ctx, zone, plan); err != nil {
		t.Fatal(err)
	}
	if err := c.Verify(ctx, zone, plan); err != nil {
		t.Fatal(err)
	}
	vals, _, labels, _ := sim.RRSet("100001", "_hashi-test", "TXT")
	if len(vals) != 1 || vals[0] != `"say \"hi\""` || labels[OwnerLabel] != "inst-1" {
		t.Errorf("TXT stored as %q labels %v", vals, labels)
	}

	observed := func() []dns.RRSet {
		all, err := c.ListRRSets(ctx, zone)
		if err != nil {
			t.Fatal(err)
		}
		var out []dns.RRSet
		for _, r := range all {
			if r.Name == "_hashi-test" {
				out = append(out, r)
			}
		}
		return out
	}
	if p, _ := c.Plan(ctx, zone, desired, observed()); len(p.Changes) != 0 {
		t.Fatalf("second plan not empty: %+v", p.Changes)
	}
	a2 := a
	a2.Values = []string{"192.0.2.11"}
	plan, _ = c.Plan(ctx, zone, []dns.RRSet{a2}, observed())
	if len(plan.Changes) != 2 || plan.Changes[0].Op != dns.OpDelete || plan.Changes[1].Op != dns.OpUpdate {
		t.Fatalf("plan %+v", plan.Changes)
	}
	if err := c.Apply(ctx, zone, plan); err != nil {
		t.Fatal(err)
	}
	if err := c.Verify(ctx, zone, plan); err != nil {
		t.Fatal(err)
	}

	// Verify notices a mismatch.
	a3 := a2
	a3.Values = []string{"192.0.2.99"}
	if err := c.Verify(ctx, zone, dns.ProviderPlan{Changes: []dns.Change{{Op: dns.OpUpdate, Before: &a2, After: &a3}}}); !errors.Is(err, dns.ErrVerify) {
		t.Errorf("verify mismatch: %v", err)
	}
}

func noSleep(context.Context, time.Duration) error { return nil }
