package httpapi

import (
	"testing"
	"time"

	hsync "git.juzo.io/juzo/hashi/internal/sync"
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
