// Package edge is forward auth (06 §2): it resolves the service, runs the rule list and answers Traefik. Every input lives
// in an in-memory snapshot, refreshed on changes and every 30 s. Stage A's state lives in internal/security; its static
// half, the folded allow and block partition, is part of the snapshot.
package edge

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go4.org/netipx"

	"git.juzo.io/juzo/hashi/internal/ipsets"
	"git.juzo.io/juzo/hashi/internal/rules"
	"git.juzo.io/juzo/hashi/internal/services"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/store"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

// JobKind refreshes the snapshot (06 §2.2) and drops ended sessions.
const (
	JobKind     = "edge.refresh"
	JobInterval = 30 * time.Second
)

// Compiled is one IP set as forward auth tests it.
type Compiled struct {
	ips *netipx.IPSet
	geo []ipsets.Geo
}

// GeoLookup reports what the database knows about an address. ok is false without a database.
type GeoLookup func(a netip.Addr) (country, region string, asn uint, ok bool)

type serviceView struct {
	ID, Name string
	List     []rules.Flat
	// Challenge marks the CAPTCHA challenge service: Stage A runs A1 only and only the flow's paths pass (06 §11).
	Challenge bool
	// WAF and System decide the service's WAF per path (services.ResolveWAF).
	WAF    services.WAF
	System bool
}

type snapshot struct {
	byHost map[string]serviceView
	byID   map[string]serviceView
	sets   map[string]Compiled
	allID  string
	// allow and block are the firewall's folded partition (05 §4.2): the same ranges the kernel's sets receive.
	allow, block *netipx.IPSet
}

// Partition returns the folded allow and block ranges; app wires it to the firewall's folding.
type Partition func(ctx context.Context) (allow, block []netip.Prefix, err error)

// Engine answers rule decisions from the current snapshot.
type Engine struct {
	st   *store.Store
	sets *ipsets.Service
	log  *slog.Logger
	geo  atomic.Pointer[GeoLookup]
	part atomic.Pointer[Partition]
	cur  atomic.Pointer[snapshot]
	// Rebuilds run one at a time. Each call takes a ticket; a build covers every ticket taken before it started
	// reading, so a caller returns only after a build that began after its call (bug 32), and callers that queue
	// behind a running build share the next one.
	buildMu sync.Mutex
	tickets atomic.Uint64
	built   uint64 // highest ticket a finished build covers; guarded by buildMu
}

// NewEngine returns an engine without a snapshot; call Rebuild before serving.
func NewEngine(st *store.Store, sets *ipsets.Service, log *slog.Logger) *Engine {
	return &Engine{st: st, sets: sets, log: log}
}

// UseGeo sets the GeoIP lookup rules with countries or networks ask.
func (e *Engine) UseGeo(l GeoLookup) { e.geo.Store(&l) }

// UsePartition sets the source of the folded allow and block ranges. Without one nothing is allowed or blocked.
func (e *Engine) UsePartition(p Partition) { e.part.Store(&p) }

// Allowed reports whether a is in the folded allow ranges: never challenged, banned or counted (invariant 20).
func (e *Engine) Allowed(a netip.Addr) bool {
	s := e.cur.Load()
	return s != nil && s.allow != nil && s.allow.Contains(a.Unmap())
}

// StaticBlocked reports whether a is in the folded block ranges (A1's static half).
func (e *Engine) StaticBlocked(a netip.Addr) bool {
	s := e.cur.Load()
	return s != nil && s.block != nil && s.block.Contains(a.Unmap())
}

func ipSet(ps []netip.Prefix) (*netipx.IPSet, error) {
	var b netipx.IPSetBuilder
	for _, p := range ps {
		b.AddPrefix(p)
	}
	return b.IPSet()
}

func isHTTP(kind string) bool { return kind == "http" || kind == "https" || kind == "h2c" }

// Rebuild replaces the snapshot. On an error the previous one stays in force.
func (e *Engine) Rebuild(ctx context.Context) error {
	ticket := e.tickets.Add(1)
	e.buildMu.Lock()
	defer e.buildMu.Unlock()
	if e.built >= ticket {
		return nil // a build that started after this call has finished
	}
	covers := e.tickets.Load()
	if err := e.build(ctx); err != nil {
		return err
	}
	e.built = covers
	return nil
}

// build reads rules, services, sets and settings and installs the new snapshot.
func (e *Engine) build(ctx context.Context) error {
	g, err := rules.Load(ctx, e.st.R)
	if err != nil {
		return err
	}
	svcs, err := e.st.R.ListServices(ctx)
	if err != nil {
		return fmt.Errorf("list services: %w", err)
	}
	next := &snapshot{byHost: map[string]serviceView{}, byID: map[string]serviceView{}, sets: map[string]Compiled{}}
	all, err := e.st.R.GetIPSetBySystemKey(ctx, sql.NullString{String: ipsets.SystemAll, Valid: true})
	if err != nil {
		return fmt.Errorf("the all set: %w", err)
	}
	next.allID = all.ID
	if p := e.part.Load(); p != nil {
		allow, block, err := (*p)(ctx)
		if err != nil {
			return fmt.Errorf("folded allow and block: %w", err)
		}
		if next.allow, err = ipSet(allow); err != nil {
			return err
		}
		if next.block, err = ipSet(block); err != nil {
			return err
		}
	}
	capCfg, err := settings.Get[settings.Captcha](ctx, e.st.R)
	if err != nil {
		return err
	}
	roots := map[string]string{}
	for _, s := range g {
		if s.Kind == rules.KindServiceRoot {
			roots[s.ServiceID] = s.ID
		}
	}
	for _, row := range svcs {
		if !isHTTP(row.Kind) {
			continue
		}
		root, ok := roots[row.ID]
		if !ok {
			// Between a system service's creation and the job that gives it a root (seconds): the admin page has its
			// own login and must stay reachable. Any other service gets its root in the transaction that creates it.
			if row.Origin == "system" && row.HostName != "" {
				next.byHost[strings.ToLower(row.HostName)] = serviceView{ID: row.ID, Name: row.Name, List: []rules.Flat{{Entry: rules.Entry{AppliesTo: next.allID, Action: rules.ActionNone, Fallback: true}}}}
				next.sets[next.allID] = Compiled{}
			}
			continue
		}
		v := serviceView{ID: row.ID, Name: row.Name, List: g.Flatten(root, row.Origin == "system"),
			Challenge: row.Origin == "system" && row.OwnerFeature == "captcha", System: row.Origin == "system"}
		if err := json.Unmarshal([]byte(row.WafJson), &v.WAF); err != nil {
			return fmt.Errorf("service %s WAF: %w", row.ID, err)
		}
		if v.Challenge || row.ID == capCfg.AdminServiceID {
			// No CAPTCHA in front of CAPTCHA (06 §11): pass_captcha items, also from the global set, are left out.
			v.List = slices.DeleteFunc(slices.Clone(v.List), func(f rules.Flat) bool { return f.Entry.Action == rules.ActionCaptcha })
		}
		next.byID[row.ID] = v
		if row.HostName != "" {
			next.byHost[strings.ToLower(row.HostName)] = v
		}
		for _, f := range v.List {
			if _, ok := next.sets[f.Entry.AppliesTo]; ok {
				continue
			}
			c, err := e.compile(ctx, e.st.R, f.Entry.AppliesTo)
			if err != nil {
				return err
			}
			next.sets[f.Entry.AppliesTo] = c
		}
	}
	e.cur.Store(next)
	return nil
}

func (e *Engine) compile(ctx context.Context, q *db.Queries, id string) (Compiled, error) {
	res, err := e.sets.Compile(ctx, q, id)
	if err != nil {
		return Compiled{}, fmt.Errorf("compile set %s: %w", id, err)
	}
	ips, err := ipSet(append(append([]netip.Prefix{}, res.V4...), res.V6...))
	if err != nil {
		return Compiled{}, fmt.Errorf("set %s: %w", id, err)
	}
	return Compiled{ips: ips, geo: res.Geo}, nil
}

// Refresh is the periodic job.
func (e *Engine) Refresh(ctx context.Context) (string, error) {
	return "", e.Rebuild(ctx)
}

// in reports whether a is in the compiled set: its intervals or, at this layer, its geo predicates (05a §4.1).
func (e *Engine) in(s *snapshot, id string, a netip.Addr) bool {
	c, ok := s.sets[id]
	if !ok {
		return false
	}
	if c.ips != nil && c.ips.Contains(a) {
		return true
	}
	if len(c.geo) == 0 {
		return false
	}
	lookup := e.geo.Load()
	if lookup == nil {
		return false
	}
	country, region, asn, ok := (*lookup)(a)
	if !ok {
		return false
	}
	for _, g := range c.geo {
		if g.Matches(country, region, asn) {
			return true
		}
	}
	return false
}

// Decision is one Stage B outcome with its trace (06 §4.3).
type Decision struct {
	ServiceID string
	// Challenge: the request is for the CAPTCHA challenge service.
	Challenge bool
	// WAF and System are the service's WAF settings, resolved per path by the caller.
	WAF    services.WAF
	System bool
	Action string
	Item   *rules.Flat
	Index  int
	List   []rules.Flat
}

// DecideHost resolves the service by host and evaluates. A name no service has is denied: it may be an old name Traefik
// still routes until its plan applies, or a spelling Traefik accepts. The caller answers the health routers first.
func (e *Engine) DecideHost(host string, a netip.Addr, path string) Decision {
	s := e.cur.Load()
	if s == nil {
		return Decision{Action: rules.ActionDeny, Index: -1} // never serve from nothing; Rebuild runs before the listener
	}
	v, ok := s.byHost[strings.TrimSuffix(strings.ToLower(host), ".")] // Traefik's Host() matches the trailing-dot form too
	if !ok {
		return Decision{Action: rules.ActionDeny, Index: -1}
	}
	return e.decide(s, v, a, path)
}

// DecideService evaluates for one service (the "try a request" trace).
func (e *Engine) DecideService(serviceID string, a netip.Addr, path string) (Decision, bool) {
	s := e.cur.Load()
	if s == nil {
		return Decision{}, false
	}
	v, ok := s.byID[serviceID]
	if !ok {
		return Decision{}, false
	}
	return e.decide(s, v, a, path), true
}

func (e *Engine) decide(s *snapshot, v serviceView, a netip.Addr, path string) Decision {
	d := Decision{ServiceID: v.ID, Challenge: v.Challenge, WAF: v.WAF, System: v.System, List: v.List, Index: -1}
	d.Index = rules.Evaluate(v.List, NormalizePath(path), func(id string) bool { return e.in(s, id, a) })
	if d.Index < 0 {
		// The fallback covers everyone, so this means an address no compiled set contains (none): nothing matched means denied.
		d.Action = rules.ActionDeny
		return d
	}
	d.Item, d.Action = &v.List[d.Index], v.List[d.Index].Entry.Action
	return d
}
