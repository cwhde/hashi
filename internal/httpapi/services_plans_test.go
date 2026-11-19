package httpapi

import (
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/firewall"
	"git.juzo.io/juzo/hashi/internal/services"
	hsync "git.juzo.io/juzo/hashi/internal/sync"
	"git.juzo.io/juzo/hashi/internal/traefik"
)

// Bug 76: a service save said "1 plan needs your approval" for a firewall plan that had been waiting since before the
// save. Only plans the save made, or that it ran, belong in its answer.
func TestServiceSaveCountsOnlyItsOwnPlans(t *testing.T) {
	start := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		p    hsync.Plan
		want bool
	}{
		{"waiting from before", hsync.Plan{Status: hsync.StatusPending, CreatedAt: start.Add(-time.Hour)}, false},
		{"made by this save, waiting", hsync.Plan{Status: hsync.StatusPending, CreatedAt: start}, true},
		{"made and applied by this save", hsync.Plan{Status: hsync.StatusApplied, CreatedAt: start.Add(time.Second)}, true},
	}
	for _, c := range cases {
		if got := madeBy(&c.p, start); got != c.want {
			t.Errorf("%s: madeBy = %v, want %v", c.name, got, c.want)
		}
	}
}

// Bug 77: a TCP/UDP service said "Published" while both hosts' plans still waited and nothing had reached them.
func TestServiceWaitsWhileItsPlansAreOpen(t *testing.T) {
	tcp := services.Service{Kind: "tcp", Enabled: true, EntryHostID: "fw1"}
	web := services.Service{Kind: "http", Enabled: true}
	off := services.Service{Kind: "tcp", Enabled: false, EntryHostID: "fw1"}
	bad := services.Service{Kind: "http", Enabled: true, InvalidReason: "no target"}
	plan := func(sub, target string) *hsync.Plan { return &hsync.Plan{Subsystem: sub, Target: target} }

	cases := []struct {
		name string
		open []*hsync.Plan
		sv   services.Service
		want bool
	}{
		{"nothing open", nil, tcp, false},
		{"entry host's firewall plan open", []*hsync.Plan{plan(firewall.Subsystem, "fw1")}, tcp, true},
		{"another host's firewall plan open", []*hsync.Plan{plan(firewall.Subsystem, "fw2")}, tcp, false},
		{"HTTP ignores firewall plans", []*hsync.Plan{plan(firewall.Subsystem, "fw1")}, web, false},
		{"Traefik plan open", []*hsync.Plan{plan(traefik.Subsystem, "t1")}, web, true},
		{"Traefik plan open, TCP", []*hsync.Plan{plan(traefik.Subsystem, "t1")}, tcp, true},
		{"off is never waiting", []*hsync.Plan{plan(traefik.Subsystem, "t1")}, off, false},
		{"invalid is never waiting", []*hsync.Plan{plan(traefik.Subsystem, "t1")}, bad, false},
	}
	for _, c := range cases {
		if got := waitingFor(c.open)(c.sv); got != c.want {
			t.Errorf("%s: waiting = %v, want %v", c.name, got, c.want)
		}
	}
}
