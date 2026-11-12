package edge

import (
	"context"
	"log/slog"
	"net/netip"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/ipsets"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
)

// Bug 32: a change saved while a rebuild is already reading must not be answered with that older snapshot. Rebuild
// returns only after a build that started after the call (CI runs 1196, 1207: a rule change applied up to 30 s late).
func TestRebuildAfterAChangeSeesTheChange(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	e := NewEngine(st, ipsets.NewService(st, audit.New(nil), slog.New(slog.DiscardHandler), nil), slog.New(slog.DiscardHandler))
	var allow atomic.Pointer[netip.Prefix]
	before := netip.MustParsePrefix("192.0.2.1/32")
	after := netip.MustParsePrefix("198.51.100.1/32")
	allow.Store(&before)
	reading, release := make(chan struct{}), make(chan struct{})
	var first sync.Once
	e.UsePartition(func(context.Context) ([]netip.Prefix, []netip.Prefix, error) {
		p := *allow.Load() // what this build read
		first.Do(func() { close(reading); <-release })
		return []netip.Prefix{p}, nil, nil
	})
	done := make(chan error, 1)
	go func() { done <- e.Rebuild(ctx) }() // the periodic refresh, caught mid-read
	<-reading
	allow.Store(&after) // the admin saves a change
	saved := make(chan error, 1)
	go func() { saved <- e.Rebuild(ctx) }() // and asks for a rebuild
	time.Sleep(50 * time.Millisecond)       // let it join the running build, if it would
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := <-saved; err != nil {
		t.Fatal(err)
	}
	if !e.Allowed(after.Addr()) {
		t.Fatal("the rebuild after the change returned the snapshot read before it")
	}
}
