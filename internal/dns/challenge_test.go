package dns_test

import (
	"errors"
	"slices"
	"testing"

	"git.juzo.io/juzo/hashi/internal/dns"
)

// Invariant 1, 03 §5.3: challenge values are added and removed one at a time, so another client's value survives; challenge names
// never reach the import table and cannot be entries.
func TestInvariant_1_ChallengeValuesLeaveOtherWritersAlone(t *testing.T) {
	f := newWriteFixture(t)
	ctx := t.Context()
	ch, err := f.svc.Challenger(ctx, f.zone)
	if err != nil {
		t.Fatal(err)
	}
	if len(ch.Nameservers()) == 0 {
		t.Error("no authoritative nameservers from the snapshot")
	}
	name := "_acme-challenge.example.com"
	for _, v := range []string{"other client", "hashi root", "hashi wildcard"} {
		if err := ch.Present(ctx, name, v); err != nil {
			t.Fatal(err)
		}
	}
	if err := ch.Present(ctx, name, "hashi root"); err != nil {
		t.Errorf("present twice: %v", err)
	}
	vals, ttl, _, ok := f.sim.RRSet("100001", "_acme-challenge", "TXT")
	if !ok || len(vals) != 3 || ttl == nil || *ttl != dns.ChallengeTTL {
		t.Fatalf("after present: %q ttl %v", vals, ttl)
	}

	if _, err := f.svc.Refresh(ctx, f.zone); err != nil {
		t.Fatal(err)
	}
	_, rows, err := f.svc.ImportTable(ctx, f.zone)
	if err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(rows, func(r dns.ImportRow) bool { return dns.IsChallenge(r.Name) }) {
		t.Error("challenge RRSet offered on the import table")
	}
	if _, err := f.svc.CreateRecord(ctx, "admin", "", f.zone, dns.RecordInput{Name: "_acme-challenge.app", Type: "TXT",
		Values: []string{"x"}}); !errors.Is(err, dns.ErrInvalid) {
		t.Errorf("challenge name as an entry: %v", err)
	}

	for _, v := range []string{"hashi root", "hashi wildcard", "hashi wildcard"} {
		if err := ch.CleanUp(ctx, name, v); err != nil {
			t.Fatal(err)
		}
	}
	if vals, _, _, _ := f.sim.RRSet("100001", "_acme-challenge", "TXT"); !slices.Equal(vals, []string{`"other client"`}) {
		t.Errorf("after cleanup: %q", vals)
	}
	for _, bad := range []string{"app.example.com", "_acme-challenge.example.org", "x._acme-challenge.example.com.evil"} {
		if err := ch.Present(ctx, bad, "v"); err == nil {
			t.Errorf("%s accepted", bad)
		}
	}
}
