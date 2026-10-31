package ipsets

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"slices"
	"sort"
	"sync"
	"time"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/store"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

// DNSRefresh is how long a dns item's answers are kept. The standard library resolver does not expose record TTLs;
// a fixed interval inside the spec's 60 s – 1 h window is used instead (05a §2).
const DNSRefresh = 5 * time.Minute

// JobKind refreshes dns items and drops expired elements; JobInterval is its period.
const (
	JobKind     = "ipsets.refresh"
	JobInterval = time.Minute
)

// Debounce batches source changes before a recompile (05a §4.2, 01 §5.4).
const Debounce = 2 * time.Second

// LookupFunc resolves a name to addresses (net.Resolver.LookupNetIP in production).
type LookupFunc func(ctx context.Context, fqdn string) ([]netip.Addr, error)

// PulseAddrs returns a Pulse agent's addresses in a scope.
type PulseAddrs func(ctx context.Context, agentID, scope, selector string) ([]netip.Addr, error)

// HostAddrs returns a host's addresses in a scope; nil until firewall hosts exist.
type HostAddrs func(ctx context.Context, hostID, scope string) ([]netip.Addr, error)

// Subscriber is told which sets' compiled content changed.
type Subscriber func(ctx context.Context, changed []string)

// Service compiles sets, caches dns answers and notifies consumers when a compiled set changes (05a §4.2).
type Service struct {
	st     *store.Store
	al     *audit.Log
	log    *slog.Logger
	lookup LookupFunc
	now    func() time.Time

	mu      sync.Mutex
	hosts   HostAddrs
	pulse   PulseAddrs
	lookupP PulseLookup
	dns     map[string]dnsEntry
	subs    []Subscriber
	sources []ConsumerSource
	lookupH HostLookup
	timer   *time.Timer
}

type dnsEntry struct {
	addrs   []netip.Addr
	err     error
	fetched time.Time
}

// NewService returns a service; lookup nil means the system resolver.
func NewService(st *store.Store, al *audit.Log, log *slog.Logger, lookup LookupFunc) *Service {
	if lookup == nil {
		lookup = func(ctx context.Context, fqdn string) ([]netip.Addr, error) {
			return net.DefaultResolver.LookupNetIP(ctx, "ip", fqdn)
		}
	}
	return &Service{st: st, al: al, log: log, lookup: lookup, now: time.Now, dns: map[string]dnsEntry{}}
}

// Subscribe registers a consumer for compiled-set changes.
func (s *Service) Subscribe(f Subscriber) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subs = append(s.subs, f)
}

// AddConsumers registers a feature's consumer list (used by delete checks, "used by" and target checks).
func (s *Service) AddConsumers(src ConsumerSource) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sources = append(s.sources, src)
}

// SetHosts wires host items to the firewall feature.
func (s *Service) SetHosts(addrs HostAddrs, lookup HostLookup) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hosts, s.lookupH = addrs, lookup
}

// SetPulse wires pulse items to the Pulse feature.
func (s *Service) SetPulse(addrs PulseAddrs, exists PulseLookup) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pulse, s.lookupP = addrs, exists
}

// Impact names the changed sets and everything using them, for Activity (08 §4).
func (s *Service) Impact(ctx context.Context, changed []string) (sets, consumers []string) {
	g, err := LoadGraph(ctx, s.st.R)
	if err != nil {
		return nil, nil
	}
	// A set holding a changed set changes with it.
	seen := map[string]bool{}
	var all []string
	var up func(string)
	up = func(id string) {
		if seen[id] {
			return
		}
		seen[id] = true
		all = append(all, id)
		for _, pid := range sortedIDs(g) {
			if slices.ContainsFunc(g[pid].Elements, func(e Element) bool { r, ok := e.Item.(SetRef); return ok && r.SetID == id }) {
				up(pid)
			}
		}
	}
	for _, id := range changed {
		up(id)
	}
	for _, id := range all {
		if x, ok := g[id]; ok {
			sets = append(sets, x.Name)
		}
		for _, src := range s.Sources() {
			c, err := src(ctx, s.st.R, id)
			if err != nil {
				continue
			}
			for _, u := range c {
				consumers = append(consumers, u.Name)
			}
		}
	}
	return sets, consumers
}

// Editor returns an element editor that knows every registered consumer and the hosts.
func (s *Service) Editor() Editor {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Editor{Sources: append([]ConsumerSource(nil), s.sources...), Hosts: s.lookupH, Agents: s.lookupP}
}

// Sources returns the registered consumer sources.
func (s *Service) Sources() []ConsumerSource {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]ConsumerSource(nil), s.sources...)
}

// Resolver returns a resolver for one compile: dns answers from the cache (resolved on a miss), hosts from the
// firewall feature.
func (s *Service) Resolver(ctx context.Context) Resolver { return &resolver{s: s, ctx: ctx} }

type resolver struct {
	s   *Service
	ctx context.Context
}

func (r *resolver) Host(hostID, scope string) ([]netip.Addr, error) {
	r.s.mu.Lock()
	h := r.s.hosts
	r.s.mu.Unlock()
	if h == nil {
		return nil, fmt.Errorf("no firewall or edge hosts are registered")
	}
	return h(r.ctx, hostID, scope)
}

func (r *resolver) Pulse(agentID, scope, selector string) ([]netip.Addr, error) {
	r.s.mu.Lock()
	p := r.s.pulse
	r.s.mu.Unlock()
	if p == nil {
		return nil, fmt.Errorf("no Pulse agents are registered")
	}
	return p(r.ctx, agentID, scope, selector)
}

func (r *resolver) DNS(fqdn string) ([]netip.Addr, error) {
	r.s.mu.Lock()
	e, ok := r.s.dns[fqdn]
	r.s.mu.Unlock()
	if !ok {
		e = r.s.resolve(r.ctx, fqdn)
	}
	return e.addrs, e.err
}

func (s *Service) resolve(ctx context.Context, fqdn string) dnsEntry {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	addrs, err := s.lookup(ctx, fqdn)
	for i := range addrs {
		addrs[i] = addrs[i].Unmap()
	}
	sort.Slice(addrs, func(i, j int) bool { return addrs[i].Less(addrs[j]) })
	e := dnsEntry{addrs: addrs, err: err, fetched: s.now()}
	s.mu.Lock()
	s.dns[fqdn] = e
	s.mu.Unlock()
	return e
}

// Compile compiles one set with the live resolver.
func (s *Service) Compile(ctx context.Context, q *db.Queries, id string) (Result, error) {
	g, err := LoadGraph(ctx, q)
	if err != nil {
		return Result{}, err
	}
	if _, ok := g[id]; !ok {
		return Result{}, ErrNotFound
	}
	return g.Compile(id, s.Resolver(ctx), s.now()), nil
}

// Changed schedules a debounced recompile after a source change (an edit, a host address, a dns answer).
func (s *Service) Changed(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.timer != nil {
		s.timer.Stop()
	}
	ctx = context.WithoutCancel(ctx)
	s.timer = time.AfterFunc(Debounce, func() {
		if _, err := s.Recompile(ctx); err != nil {
			s.log.Warn("recompile IP sets", "err", err)
		}
	})
}

// Recompile compiles every set, stores the results and notifies subscribers of the sets whose content changed.
func (s *Service) Recompile(ctx context.Context) ([]string, error) {
	g, err := LoadGraph(ctx, s.st.R)
	if err != nil {
		return nil, err
	}
	rows, err := s.st.R.ListIPSets(ctx)
	if err != nil {
		return nil, err
	}
	prev := make(map[string]db.IpSet, len(rows))
	for _, r := range rows {
		prev[r.ID] = r
	}
	r, now := s.Resolver(ctx), s.now()
	var changed []string
	err = s.st.Tx(ctx, func(q *db.Queries) error {
		for _, id := range sortedIDs(g) {
			res := g.Compile(id, r, now)
			problems, err := json.Marshal(res.Problems)
			if err != nil {
				return err
			}
			p := prev[id]
			if p.CompiledHash == res.Hash() && p.ResolutionStatus == res.Status && p.ResolutionProblemsJson == string(problems) {
				continue
			}
			if p.CompiledHash != res.Hash() {
				changed = append(changed, id)
			}
			if err := q.SetIPSetCompiled(ctx, db.SetIPSetCompiledParams{CompiledHash: res.Hash(), ResolutionStatus: res.Status,
				ResolutionProblemsJson: string(problems), CompiledAt: nullMs(now), ID: id}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil || len(changed) == 0 {
		return changed, err
	}
	s.mu.Lock()
	subs := append([]Subscriber(nil), s.subs...)
	s.mu.Unlock()
	for _, f := range subs {
		f(ctx, changed)
	}
	return changed, nil
}

// Refresh is the job: re-resolve dns answers older than DNSRefresh, drop expired elements (one audit entry each),
// then recompile.
func (s *Service) Refresh(ctx context.Context) (string, error) {
	now := s.now()
	s.mu.Lock()
	var stale []string
	for name, e := range s.dns {
		if now.Sub(e.fetched) >= DNSRefresh {
			stale = append(stale, name)
		}
	}
	s.mu.Unlock()
	for _, name := range stale {
		s.resolve(ctx, name)
	}
	expired := 0
	err := s.st.Tx(ctx, func(q *db.Queries) error {
		els, err := q.ListAllIPSetElements(ctx)
		if err != nil {
			return err
		}
		for _, r := range els {
			if !r.ExpiresAt.Valid || r.ExpiresAt.Int64 > now.UnixMilli() {
				continue
			}
			if err := q.DeleteIPSetElement(ctx, r.ID); err != nil {
				return err
			}
			expired++
			if err := s.al.Record(ctx, q, audit.Event{Actor: audit.System, Action: "ip_set.element.expired",
				Target: "ip_set:" + r.IpSetID, Outcome: audit.Success,
				Detail: map[string]any{"element_id": r.ID, "type": r.Type, "note": r.Note}}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	changed, err := s.Recompile(ctx)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d dns name(s) refreshed, %d element(s) expired, %d set(s) changed", len(stale), expired, len(changed)), nil
}

func sortedIDs(g Graph) []string {
	ids := make([]string, 0, len(g))
	for id := range g {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func nullMs(t time.Time) sql.NullInt64 { return sql.NullInt64{Int64: t.UnixMilli(), Valid: true} }
