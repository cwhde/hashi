package httpapi

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Seen on prod 2026-11-20: one AdGuard instance took 15 s to list its rewrites, and every search waited for it. Search
// now answers from a copy at once; the refresh runs behind it, and an instance that fails keeps what it had.
func TestSearchNeverWaitsForAdGuard(t *testing.T) {
	ctx := context.Background()
	slow := make(chan struct{})
	failing := false
	src := rewriteSource{
		conns: func(context.Context) ([]string, error) { return []string{"fast", "slow"}, nil },
		read: func(ctx context.Context, id string) ([]searchHit, error) {
			if id == "slow" {
				if failing {
					return nil, errors.New("timeout")
				}
				<-slow
			}
			return []searchHit{{title: id + ".example.com"}}, nil
		},
	}
	var c rewriteCache
	refreshed := make(chan struct{}, 4)
	done := func() { refreshed <- struct{}{} }
	now := time.Now()

	start := time.Now()
	if got := c.hits(ctx, now, src, done); len(got) != 0 {
		t.Fatalf("the first search has no copy yet: %+v", got)
	}
	if d := time.Since(start); d > 100*time.Millisecond {
		t.Fatalf("search waited %v for the instances", d)
	}
	// A second search while the refresh still waits on the slow instance answers at once and starts no other refresh.
	_ = c.hits(ctx, now, src, done)
	close(slow)
	<-refreshed
	select {
	case <-refreshed:
		t.Fatal("two refreshes ran at once")
	case <-time.After(50 * time.Millisecond):
	}
	if got := c.hits(ctx, now, src, done); len(got) != 2 || got[0].title != "fast.example.com" || got[1].title != "slow.example.com" {
		t.Fatalf("after the refresh: %+v", got)
	}

	// Old enough to refresh again; this time the slow instance fails and keeps its rewrites.
	failing = true
	_ = c.hits(ctx, now.Add(2*rewritesFresh), src, done)
	<-refreshed
	if got := c.hits(ctx, now, src, done); len(got) != 2 {
		t.Fatalf("a failing instance lost its rewrites: %+v", got)
	}
}
