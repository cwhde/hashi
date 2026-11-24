package monitor

import (
	"context"
	"fmt"
	"math/rand/v2"
	"net"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"time"
)

// How long a looked-up name is kept, and how fast new lookups leave (bug 161). A check round used to resolve every name
// at once, A and AAAA, every 10 s; behind AdGuard's default limit (20 queries a second per /24) the overflow was dropped
// and timed out. Lives are spread so names learned together don't expire together; 8 lookups a second are 16 queries.
const (
	nameTTLMin    = time.Minute
	nameTTLJitter = 2 * time.Minute
	lookupGap     = 125 * time.Millisecond
)

// Names caches the addresses of the names checks connect to, shares one lookup between checks that ask at the same
// time, and paces lookups. Failures are not kept: the next check asks again. DNS checks never use it: asking is their job.
type Names struct {
	resolve func(ctx context.Context, host string) ([]netip.Addr, error)
	gap     time.Duration
	now     func() time.Time

	mu       sync.Mutex
	entries  map[string]nameEntry
	inflight map[string]*nameCall
	next     time.Time // when the next paced lookup may leave
}

type nameEntry struct {
	addrs []netip.Addr
	until time.Time
}

type nameCall struct {
	done  chan struct{}
	addrs []netip.Addr
	err   error
}

// NewNames resolves through the system resolver.
func NewNames() *Names {
	return newNames(func(ctx context.Context, host string) ([]netip.Addr, error) {
		return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	}, lookupGap)
}

// Public resolvers for the public checks (07 §2): they see what visitors see, never the internal rewrites.
var publicResolvers = []string{"1.1.1.1:53", "9.9.9.9:53"}

// NewPublicNames resolves through public DNS, the second server when the first fails.
func NewPublicNames() *Names {
	return newNames(func(ctx context.Context, host string) ([]netip.Addr, error) {
		var last error
		for _, server := range publicResolvers {
			r := &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, server)
			}}
			addrs, err := r.LookupNetIP(ctx, "ip", host)
			if err == nil {
				return addrs, nil
			}
			last = err
		}
		return nil, last
	}, lookupGap)
}

func newNames(resolve func(context.Context, string) ([]netip.Addr, error), gap time.Duration) *Names {
	return &Names{resolve: resolve, gap: gap, now: time.Now, entries: map[string]nameEntry{}, inflight: map[string]*nameCall{}}
}

func (n *Names) lookup(ctx context.Context, host string) ([]netip.Addr, error) {
	n.mu.Lock()
	if e, ok := n.entries[host]; ok && n.now().Before(e.until) {
		n.mu.Unlock()
		return e.addrs, nil
	}
	if c, ok := n.inflight[host]; ok {
		n.mu.Unlock()
		select {
		case <-c.done:
			return c.addrs, c.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	c := &nameCall{done: make(chan struct{})}
	n.inflight[host] = c
	wait := n.reserve()
	n.mu.Unlock()

	c.addrs, c.err = n.paced(ctx, host, wait)
	n.mu.Lock()
	delete(n.inflight, host)
	if c.err == nil {
		now := n.now()
		for h, e := range n.entries {
			if !now.Before(e.until) {
				delete(n.entries, h)
			}
		}
		n.entries[host] = nameEntry{addrs: c.addrs, until: now.Add(nameTTLMin + rand.N(nameTTLJitter))} // #nosec G404 -- spreads expiry, not a secret
	}
	n.mu.Unlock()
	close(c.done)
	return c.addrs, c.err
}

// paced waits for this lookup's slot, then asks.
func (n *Names) paced(ctx context.Context, host string, wait time.Duration) ([]netip.Addr, error) {
	if wait > 0 {
		t := time.NewTimer(wait)
		defer t.Stop()
		select {
		case <-t.C:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	addrs, err := n.resolve(ctx, host)
	if err == nil && len(addrs) == 0 {
		err = fmt.Errorf("lookup %s: no addresses", host)
	}
	return addrs, err
}

// reserve takes the next lookup slot and says how long to wait for it. Called with mu held.
func (n *Names) reserve() time.Duration {
	if n.gap <= 0 {
		return 0
	}
	now := time.Now()
	start := now
	if n.next.After(now) {
		start = n.next
	}
	n.next = start.Add(n.gap)
	return start.Sub(now)
}

// dialer connects for one check: names through the public resolver for a public check, else the cache, and only to
// addresses of the check's family. Like net.Dialer, it tries the addresses in the resolver's order and gives each a
// share of the time left (at least 2 s), so one dead address can't use up the whole check.
func (e Executor) dialer(c Check) func(ctx context.Context, network, addr string) (net.Conn, error) {
	names := e.Names
	if c.Public && e.PublicNames != nil {
		names = e.PublicNames
	}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		d := &net.Dialer{}
		if c.Family != "" {
			network = strings.TrimRight(network, "46") + c.Family // tcp4, udp6, …: the system resolver keeps to it too
		}
		host, port, err := net.SplitHostPort(addr)
		if err != nil || names == nil {
			return d.DialContext(ctx, network, addr)
		}
		if a, err := netip.ParseAddr(host); err == nil {
			if !family(a, c.Family) {
				return nil, fmt.Errorf("%s is not an IPv%s address", host, c.Family)
			}
			return d.DialContext(ctx, network, addr)
		}
		all, err := names.lookup(ctx, host)
		if err != nil {
			return nil, err
		}
		addrs := slices.DeleteFunc(slices.Clone(all), func(a netip.Addr) bool { return !family(a, c.Family) })
		if len(addrs) == 0 {
			return nil, fmt.Errorf("%s has no IPv%s address", host, c.Family)
		}
		var last error
		for i, a := range addrs {
			actx := ctx
			if dl, ok := ctx.Deadline(); ok && i < len(addrs)-1 {
				share := max(time.Until(dl)/time.Duration(len(addrs)-i), 2*time.Second)
				var cancel context.CancelFunc
				actx, cancel = context.WithTimeout(ctx, share)
				defer cancel()
			}
			conn, err := d.DialContext(actx, network, net.JoinHostPort(a.String(), port))
			if err == nil {
				return conn, nil
			}
			last = err
		}
		return nil, last
	}
}

// family reports whether a is of the family "4", "6" or either ("").
func family(a netip.Addr, f string) bool {
	a = a.Unmap()
	return f == "" || (f == "4") == a.Is4()
}
