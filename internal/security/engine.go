package security

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oklog/ulid/v2"

	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/store"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

// Where an entry came from (06a §2.3).
const (
	SourceRate            = "l7_rate"
	SourceWAF             = "l7_waf"
	SourceChallengeFailed = "l7_challenge_failed"
	SourceMeter           = "l4_meter"
	SourceManual          = "manual"
)

// The two dynamic sets (seeded by migration 21).
const (
	SetChallenge = "sys-dynamic-challenge"
	SetBlocked   = "sys-dynamic-blocked"
)

// never stands for "no end": a permanent ban's quiet time never starts, so its counter never decays (06 §5.1).
var never = time.UnixMilli(math.MaxInt64 / 2)

// Entry is one address in a dynamic set.
type Entry struct {
	ElementID string
	Prefix    netip.Prefix // /32, or /64 for IPv6
	Created   time.Time
	Expires   time.Time // zero: no expiry (a permanent ban, or T_exp = 0)
	Source    string
	Reason    string
	Meter     string
	HostID    string // origin host of an L4 meter ban
	ServiceID string
	Failures  int // challenge entries: unsolved requests so far (f)
	dirty     bool
}

func (x *Entry) live(now time.Time) bool { return x.Expires.IsZero() || now.Before(x.Expires) }

// Cause describes an escalation or a new challenge for the record.
type Cause struct {
	Source, Reason, Meter, HostID, ServiceID string
	Actor                                    string // manual actions
}

// GeoLookup gives the country and ASN recorded with an offence; ok is false without a database.
type GeoLookup func(a netip.Addr) (country string, asn uint, ok bool)

// Engine holds the dynamic sets in memory (the decision path never waits for the database) and persists every state
// change. It is the only writer of the dynamic sets.
type Engine struct {
	st  *store.Store
	log *slog.Logger
	now func() time.Time

	cfg     atomic.Pointer[settings.Engine]
	allowed atomic.Pointer[func(netip.Addr) bool]
	geo     atomic.Pointer[GeoLookup]

	mu         sync.Mutex
	challenged map[netip.Prefix]*Entry
	banned     map[netip.Prefix]*Entry
	rate, waf  map[netip.Prefix]*window

	ev      *events
	buckets *buckets
	// changed is told after a dynamic set changed: the IP set recompile (rules may use these sets) and the host link.
	changed atomic.Pointer[func()]
}

// NewEngine returns an engine with the default settings; call Load before serving.
func NewEngine(st *store.Store, log *slog.Logger, now func() time.Time) *Engine {
	if now == nil {
		now = time.Now
	}
	e := &Engine{st: st, log: log, now: now, challenged: map[netip.Prefix]*Entry{}, banned: map[netip.Prefix]*Entry{},
		rate: map[netip.Prefix]*window{}, waf: map[netip.Prefix]*window{}}
	e.ev, e.buckets = newEvents(st, log, now), newBuckets()
	cfg := settings.DefaultOf[settings.Engine]()
	e.cfg.Store(&cfg)
	no := func(netip.Addr) bool { return false }
	e.allowed.Store(&no)
	return e
}

// UseSettings swaps in new engine settings.
func (e *Engine) UseSettings(c settings.Engine) { e.cfg.Store(&c) }

// Settings returns the current engine settings.
func (e *Engine) Settings() settings.Engine { return *e.cfg.Load() }

// UseAllowed sets the folded allow test (05 §4.2); an address it accepts is never challenged or banned (invariant 20).
func (e *Engine) UseAllowed(f func(netip.Addr) bool) { e.allowed.Store(&f) }

// UseGeo sets the lookup recorded with offences.
func (e *Engine) UseGeo(g GeoLookup) { e.geo.Store(&g) }

// OnChange registers the one listener told after a dynamic set changed.
func (e *Engine) OnChange(f func()) { e.changed.Store(&f) }

// Allowed reports whether a is in the folded allow ranges.
func (e *Engine) Allowed(a netip.Addr) bool { return (*e.allowed.Load())(a.Unmap()) }

func (e *Engine) notify() {
	if f := e.changed.Load(); f != nil {
		(*f)()
	}
}

// Run writes queued events until ctx ends.
func (e *Engine) Run(ctx context.Context) { e.ev.run(ctx) }

// Dropped is the number of events not written because the queue was full (06 §2.2).
func (e *Engine) Dropped() int64 { return e.ev.dropped.Load() }

// Load reads the active entries of both dynamic sets.
func (e *Engine) Load(ctx context.Context) error {
	rows, err := e.st.R.ListDynamicEntries(ctx)
	if err != nil {
		return fmt.Errorf("list dynamic entries: %w", err)
	}
	ch, bn := map[netip.Prefix]*Entry{}, map[netip.Prefix]*Entry{}
	for _, r := range rows {
		x, err := entryFromRow(r)
		if err != nil {
			e.log.Warn("dynamic entry skipped", "element", r.ID, "err", err)
			continue
		}
		if r.IpSetID == SetChallenge {
			ch[x.Prefix] = x
		} else {
			bn[x.Prefix] = x
		}
	}
	e.mu.Lock()
	e.challenged, e.banned = ch, bn
	e.mu.Unlock()
	return nil
}

func entryFromRow(r db.ListDynamicEntriesRow) (*Entry, error) {
	var p netip.Prefix
	var err error
	switch {
	case r.Address.Valid:
		var a netip.Addr
		a, err = netip.ParseAddr(r.Address.String)
		p = Key(a)
	case r.Prefix.Valid:
		p, err = netip.ParsePrefix(r.Prefix.String)
	default:
		err = errors.New("neither address nor prefix")
	}
	if err != nil {
		return nil, err
	}
	x := &Entry{ElementID: r.ID, Prefix: p, Created: time.UnixMilli(r.CreatedAt), Source: r.Source, Reason: r.Reason,
		Meter: r.Meter, HostID: r.FirewallHostID.String, ServiceID: r.ServiceID.String, Failures: int(r.FailureCount)}
	if r.ExpiresAt.Valid {
		x.Expires = time.UnixMilli(r.ExpiresAt.Int64)
	}
	return x, nil
}

// Banned reports whether a's key is in dynamic-blocked now (A1's dynamic half).
func (e *Engine) Banned(a netip.Addr) bool {
	k, now := Key(a), e.now()
	e.mu.Lock()
	defer e.mu.Unlock()
	x, ok := e.banned[k]
	return ok && x.live(now)
}

// IsChallenged reports whether a's key is in dynamic-challenge now (the challenge page's status).
func (e *Engine) IsChallenged(a netip.Addr) bool {
	k, now := Key(a), e.now()
	e.mu.Lock()
	defer e.mu.Unlock()
	x, ok := e.challenged[k]
	return ok && x.live(now)
}

// Bans returns the live entries of dynamic-blocked, for the host link.
func (e *Engine) Bans() []Entry {
	now := e.now()
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]Entry, 0, len(e.banned))
	for _, x := range e.banned {
		if x.live(now) {
			out = append(out, *x)
		}
	}
	return out
}

// Challenges returns the live entries of dynamic-challenge.
func (e *Engine) Challenges() []Entry {
	now := e.now()
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]Entry, 0, len(e.challenged))
	for _, x := range e.challenged {
		if x.live(now) {
			out = append(out, *x)
		}
	}
	return out
}

// Locate gives an address's country and ASN from GeoIP ("", 0 without a database).
func (e *Engine) Locate(a netip.Addr) (string, int64) { return e.lookup(a.Unmap()) }

func (e *Engine) lookup(a netip.Addr) (string, int64) {
	if g := e.geo.Load(); g != nil {
		if c, asn, ok := (*g)(a); ok {
			return c, int64(asn) //nolint:gosec // ASNs are 32-bit
		}
	}
	return "", 0
}

// Challenge puts a into dynamic-challenge (A2's trigger, or "challenge now"). It reports false when a is allowed or
// already challenged or banned.
func (e *Engine) Challenge(ctx context.Context, a netip.Addr, c Cause) (bool, error) {
	a = a.Unmap()
	k, now := Key(a), e.now()
	if e.Allowed(a) {
		e.ev.add(event{at: now, key: k, typ: "challenge_skipped_allowed", severity: "info", source: c.Source, reason: c.Reason, service: c.ServiceID})
		return false, nil
	}
	cfg := e.Settings()
	x := &Entry{ElementID: ulid.Make().String(), Prefix: k, Created: now, Source: c.Source, Reason: c.Reason,
		ServiceID: c.ServiceID}
	if cfg.ChallengeExpirySecs > 0 {
		x.Expires = now.Add(time.Duration(cfg.ChallengeExpirySecs) * time.Second)
	}
	e.mu.Lock()
	if old, ok := e.challenged[k]; ok && old.live(now) {
		e.mu.Unlock()
		return false, nil
	}
	if b, ok := e.banned[k]; ok && b.live(now) {
		e.mu.Unlock()
		return false, nil
	}
	old := e.challenged[k]
	e.challenged[k] = x
	e.mu.Unlock()

	country, asn := e.lookup(a)
	err := e.st.Tx(ctx, func(q *db.Queries) error {
		if old != nil {
			if err := q.DeleteDynamicElement(ctx, old.ElementID); err != nil {
				return err
			}
		}
		if err := insertEntry(ctx, q, SetChallenge, x, now); err != nil {
			return err
		}
		cnt, err := counters(ctx, q, prefixKey(k))
		if err != nil {
			return err
		}
		d := time.Duration(cfg.DecaySeconds) * time.Second
		cnt.ChallengeCount = int64(Effective(int(cnt.ChallengeCount), time.UnixMilli(cnt.ChallengeQuietSince), now, d) + 1)
		cnt.ChallengeQuietSince, cnt.LastChallengeAt = now.UnixMilli(), sql.NullInt64{Int64: now.UnixMilli(), Valid: true}
		cnt.LastSource, cnt.Country, cnt.Asn = c.Source, country, asn
		return putCounters(ctx, q, cnt, now)
	})
	if err != nil {
		e.mu.Lock()
		if e.challenged[k] == x {
			delete(e.challenged, k)
		}
		e.mu.Unlock()
		return false, fmt.Errorf("challenge %s: %w", k, err)
	}
	e.ev.add(event{at: now, key: k, typ: "challenge_entered", severity: "warning", source: c.Source, reason: c.Reason, service: c.ServiceID})
	e.notify()
	return true, nil
}

// Ban escalates a (06 §3.4): skipped when allowed, otherwise out of dynamic-challenge, c ← c+1, into dynamic-blocked for
// T_ban(c). It reports whether a ban was created.
func (e *Engine) Ban(ctx context.Context, a netip.Addr, c Cause) (bool, error) {
	a = a.Unmap()
	k, now := Key(a), e.now()
	if e.Allowed(a) {
		e.ev.add(event{at: now, key: k, typ: "ban_skipped_allowed", severity: "info", source: c.Source, reason: c.Reason, service: c.ServiceID, host: c.HostID})
		return false, nil
	}
	cfg := e.Settings()
	d := time.Duration(cfg.DecaySeconds) * time.Second
	x := &Entry{ElementID: ulid.Make().String(), Prefix: k, Created: now, Source: c.Source, Reason: c.Reason,
		Meter: c.Meter, HostID: c.HostID, ServiceID: c.ServiceID}

	e.mu.Lock()
	ch, oldBan := e.challenged[k], e.banned[k]
	delete(e.challenged, k)
	e.banned[k] = x
	e.mu.Unlock()

	country, asn := e.lookup(a)
	var count int
	var permanent bool
	err := e.st.Tx(ctx, func(q *db.Queries) error {
		for _, old := range []*Entry{ch, oldBan} {
			if old != nil {
				if err := q.DeleteDynamicElement(ctx, old.ElementID); err != nil {
					return err
				}
			}
		}
		cnt, err := counters(ctx, q, prefixKey(k))
		if err != nil {
			return err
		}
		count = Effective(int(cnt.BanCount), time.UnixMilli(cnt.BanQuietSince), now, d) + 1
		var dur time.Duration
		dur, permanent = BanParamsFrom(cfg).Duration(count)
		end := never
		if !permanent {
			end = now.Add(dur)
			x.Expires = end
		}
		if err := insertEntry(ctx, q, SetBlocked, x, now); err != nil {
			return err
		}
		hosts, err := q.ListManagedFirewallHostIDs(ctx)
		if err != nil {
			return err
		}
		for _, h := range hosts {
			if err := q.InsertDynamicEntryHost(ctx, db.InsertDynamicEntryHostParams{ElementID: x.ElementID, FirewallHostID: h, State: "pending"}); err != nil {
				return err
			}
		}
		cnt.BanCount, cnt.BanQuietSince = int64(count), end.UnixMilli()
		cnt.LastBanAt = sql.NullInt64{Int64: now.UnixMilli(), Valid: true}
		cnt.LastBanEndsAt = sql.NullInt64{Int64: end.UnixMilli(), Valid: !permanent}
		cnt.LastSource, cnt.Country, cnt.Asn = c.Source, country, asn
		return putCounters(ctx, q, cnt, now)
	})
	if err != nil {
		e.mu.Lock()
		if e.banned[k] == x {
			delete(e.banned, k)
			if oldBan != nil {
				e.banned[k] = oldBan
			}
		}
		if ch != nil {
			e.challenged[k] = ch
		}
		e.mu.Unlock()
		return false, fmt.Errorf("ban %s: %w", k, err)
	}
	reason := c.Reason
	if permanent {
		reason += " (permanent)"
	}
	e.ev.add(event{at: now, key: k, typ: "ban_created", severity: "critical", source: c.Source, reason: reason,
		service: c.ServiceID, host: c.HostID, meta: map[string]any{"ban_count": count, "permanent": permanent, "expires_at": x.Expires.UnixMilli(), "actor": c.Actor}})
	e.notify()
	return true, nil
}

// Remove ends k's entries in both dynamic sets (manual remove, or a lift). It reports whether anything ended.
func (e *Engine) Remove(ctx context.Context, k netip.Prefix, actor, reason string) (bool, error) {
	e.mu.Lock()
	ch, bn := e.challenged[k], e.banned[k]
	delete(e.challenged, k)
	delete(e.banned, k)
	e.mu.Unlock()
	if ch == nil && bn == nil {
		return false, nil
	}
	err := e.st.Tx(ctx, func(q *db.Queries) error {
		for _, x := range []*Entry{ch, bn} {
			if x != nil {
				if err := q.DeleteDynamicElement(ctx, x.ElementID); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("remove %s: %w", k, err)
	}
	e.ev.add(event{at: e.now(), key: k, typ: "dynamic_removed", severity: "info", source: SourceManual, reason: reason, meta: map[string]any{"actor": actor}})
	e.notify()
	return true, nil
}

func insertEntry(ctx context.Context, q *db.Queries, set string, x *Entry, now time.Time) error {
	pos, err := q.NextIPSetElementPosition(ctx, set)
	if err != nil {
		return err
	}
	p := db.InsertIPSetElementParams{ID: x.ElementID, IpSetID: set, Position: pos, Origin: "engine", Locked: 1,
		Note: x.Reason, CreatedBy: "engine", CreatedAt: now.UnixMilli(), UpdatedAt: now.UnixMilli()}
	if x.Prefix.Addr().Is4() {
		p.Type, p.Address, p.Family = "ip", sql.NullString{String: x.Prefix.Addr().String(), Valid: true}, sql.NullInt64{Int64: 4, Valid: true}
	} else {
		p.Type, p.Prefix, p.Family = "cidr", sql.NullString{String: x.Prefix.String(), Valid: true}, sql.NullInt64{Int64: 6, Valid: true}
	}
	if !x.Expires.IsZero() {
		p.ExpiresAt = sql.NullInt64{Int64: x.Expires.UnixMilli(), Valid: true}
	}
	if err := q.InsertIPSetElement(ctx, p); err != nil {
		return err
	}
	return q.InsertDynamicEntryState(ctx, db.InsertDynamicEntryStateParams{ElementID: x.ElementID, Source: x.Source,
		Meter: x.Meter, FirewallHostID: nullable(x.HostID), ServiceID: nullable(x.ServiceID), Reason: x.Reason,
		FailureCount: int64(x.Failures)})
}

func nullable(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }

func counters(ctx context.Context, q *db.Queries, k string) (db.AddressCounter, error) {
	c, err := q.GetAddressCounters(ctx, k)
	if errors.Is(err, sql.ErrNoRows) {
		return db.AddressCounter{Address: k}, nil
	}
	return c, err
}

func putCounters(ctx context.Context, q *db.Queries, c db.AddressCounter, now time.Time) error {
	created := c.CreatedAt
	if created == 0 {
		created = now.UnixMilli()
	}
	return q.UpsertAddressCounters(ctx, db.UpsertAddressCountersParams{Address: c.Address, BanCount: c.BanCount,
		BanQuietSince: c.BanQuietSince, ChallengeCount: c.ChallengeCount, ChallengeQuietSince: c.ChallengeQuietSince,
		LastBanAt: c.LastBanAt, LastBanEndsAt: c.LastBanEndsAt, LastChallengeAt: c.LastChallengeAt,
		LastSource: c.LastSource, Country: c.Country, Asn: c.Asn, CreatedAt: created, UpdatedAt: now.UnixMilli()})
}
