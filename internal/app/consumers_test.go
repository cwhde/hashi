package app

import (
	"context"
	"log/slog"
	"slices"
	"testing"
)

// 05a §10, 08 §7: a changed set queues exactly the dependent syncs. The firewall and connections always follow; Traefik,
// DNS, AdGuard and the pending DNS mark only when a target uses the set.
func TestChangedSetReachesEveryConsumer(t *testing.T) {
	var called []string
	mark := func(n string) func(context.Context) error {
		return func(context.Context) error { called = append(called, n); return nil }
	}
	run := func(used bool) []string {
		called = nil
		c := setConsumers{firewall: mark("firewall"), traefik: mark("traefik"), dns: mark("dns"), adguard: mark("adguard"),
			reconnect:  func(context.Context, []string) { called = append(called, "connections") },
			pendingDNS: func(context.Context, []string) error { called = append(called, "pending"); return nil },
			used:       func(context.Context, []string) (bool, error) { return used, nil },
			log:        slog.New(slog.DiscardHandler)}
		c.changed(t.Context(), []string{"s"})
		return called
	}
	if got := run(true); !slices.Equal(got, []string{"firewall", "connections", "pending", "traefik", "dns", "adguard"}) {
		t.Fatalf("used: %v", got)
	}
	if got := run(false); !slices.Equal(got, []string{"firewall", "connections"}) {
		t.Fatalf("unused: %v", got)
	}
}
