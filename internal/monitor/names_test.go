package monitor

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Bug 161: every check round resolved every name again, A and AAAA, all in the same second. Behind AdGuard (20 queries a
// second per /24 by default) the overflow was dropped and the lookups timed out: about 5,400 a day on prod. Checks of
// the same name now share one cached lookup, and lookups are paced.
func TestChecksShareOneLookupPerName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
	var lookups atomic.Int32
	names := newNames(func(ctx context.Context, host string) ([]netip.Addr, error) {
		lookups.Add(1)
		time.Sleep(20 * time.Millisecond) // the round's checks arrive while the first lookup is still out
		return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
	}, 0)
	e := Executor{Names: names}
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			if r := e.Run(t.Context(), Check{Type: TypeHTTP, Target: "http://svc.test:" + port + "/"}, 3*time.Second); !r.OK {
				t.Errorf("check through the cache: %+v", r)
			}
		})
	}
	wg.Wait()
	for _, c := range []Check{{Type: TypeTCP, Target: "svc.test:" + port}, {Type: TypeHTTP, Target: "http://svc.test:" + port + "/again"}} {
		if r := e.Run(t.Context(), c, 3*time.Second); !r.OK {
			t.Fatalf("%s: %+v", c.Type, r)
		}
	}
	if n := lookups.Load(); n != 1 {
		t.Fatalf("22 checks of one name made %d lookups, want 1", n)
	}
}

func TestNamesForgetFailuresAndExpire(t *testing.T) {
	now := time.Unix(1000, 0)
	var calls atomic.Int32
	fail := true
	n := newNames(func(context.Context, string) ([]netip.Addr, error) {
		calls.Add(1)
		if fail {
			return nil, errors.New("lookup svc.test: i/o timeout")
		}
		return []netip.Addr{netip.MustParseAddr("192.0.2.7")}, nil
	}, 0)
	n.now = func() time.Time { return now }
	if _, err := n.lookup(t.Context(), "svc.test"); err == nil {
		t.Fatal("a failed lookup must fail the check")
	}
	fail = false
	if got, err := n.lookup(t.Context(), "svc.test"); err != nil || len(got) != 1 || calls.Load() != 2 {
		t.Fatalf("a failure is not remembered: %v %v, %d calls", got, err, calls.Load())
	}
	now = now.Add(nameTTLMin - time.Second)
	if _, _ = n.lookup(t.Context(), "svc.test"); calls.Load() != 2 {
		t.Fatalf("within the minimum life: %d calls, want 2", calls.Load())
	}
	now = now.Add(nameTTLMin + nameTTLJitter)
	if _, _ = n.lookup(t.Context(), "svc.test"); calls.Load() != 3 {
		t.Fatalf("after the longest life: %d calls, want 3", calls.Load())
	}
}

// A cold start with many names must not burst: lookups leave at most one per gap.
func TestNamesPaceLookups(t *testing.T) {
	var mu sync.Mutex
	var at []time.Time
	n := newNames(func(context.Context, string) ([]netip.Addr, error) {
		mu.Lock()
		at = append(at, time.Now())
		mu.Unlock()
		return []netip.Addr{netip.MustParseAddr("192.0.2.7")}, nil
	}, 20*time.Millisecond)
	var wg sync.WaitGroup
	for _, h := range []string{"a.test", "b.test", "c.test", "d.test", "e.test"} {
		wg.Go(func() { _, _ = n.lookup(t.Context(), h) })
	}
	wg.Wait()
	if len(at) != 5 {
		t.Fatalf("%d lookups, want 5", len(at))
	}
	if span := at[len(at)-1].Sub(at[0]); span < 4*20*time.Millisecond-5*time.Millisecond {
		t.Fatalf("5 lookups within %v, want them paced 20 ms apart", span)
	}
}

func TestLiteralAddressesSkipTheCache(t *testing.T) {
	n := newNames(func(context.Context, string) ([]netip.Addr, error) {
		t.Fatal("an address needs no lookup")
		return nil, nil
	}, 0)
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	if r := (Executor{Names: n}).Run(t.Context(), Check{Type: TypeHTTP, Target: srv.URL}, 3*time.Second); !r.OK {
		t.Fatalf("%+v", r)
	}
}
