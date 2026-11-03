package security

import (
	"context"
	"database/sql"
	"errors"
	"net/netip"
	"slices"
	"strings"
	"time"

	"git.juzo.io/juzo/hashi/internal/store/db"
)

// Counter states (06 §13.1).
const (
	StateBanned     = "banned"
	StateChallenged = "challenged"
	StateNone       = "none"
)

// CounterFilter narrows the counters view. Zero values mean any; State is banned, challenged, active or all.
type CounterFilter struct {
	State                string
	MinBan, MinChallenge int
	Source, Country      string
	ASN                  int64
	Since                time.Time
	Within               netip.Prefix // rows inside this address or CIDR
	Sort                 string       // ban, challenge or last (default: last offence, newest first)
}

// CounterRow is one address with its effective counters and where it stands now.
type CounterRow struct {
	Address                string
	Country                string
	ASN                    int64
	Ban, Challenge         int
	State                  string
	Until                  time.Time // end of the ban or challenge; zero: none, or no expiry
	Failures, MaxFailures  int       // challenged: unsolved requests so far and the ban limit
	LastBan, LastChallenge time.Time
	NextBanDecrement       time.Time
	NextChallengeDecrement time.Time
	LastSource             string
}

func (r CounterRow) last() time.Time {
	if r.LastBan.After(r.LastChallenge) {
		return r.LastBan
	}
	return r.LastChallenge
}

// Counters lists the counter records matching f with their decayed values (06 §5.1), sorted.
func (e *Engine) Counters(ctx context.Context, f CounterFilter) ([]CounterRow, error) {
	var since int64
	if !f.Since.IsZero() {
		since = f.Since.UnixMilli()
	}
	rows, err := e.st.R.ListAddressCounters(ctx, db.ListAddressCountersParams{Source: f.Source, Country: f.Country, Asn: f.ASN, Since: since})
	if err != nil {
		return nil, err
	}
	cfg, now := e.Settings(), e.now()
	d := time.Duration(cfg.DecaySeconds) * time.Second
	e.mu.Lock()
	live := map[string]*Entry{}
	state := map[string]string{}
	for k, x := range e.challenged {
		if x.live(now) {
			live[prefixKey(k)], state[prefixKey(k)] = x, StateChallenged
		}
	}
	for k, x := range e.banned { // a ban outranks a challenge
		if x.live(now) {
			live[prefixKey(k)], state[prefixKey(k)] = x, StateBanned
		}
	}
	snap := map[string]Entry{}
	for k, x := range live {
		snap[k] = *x
	}
	e.mu.Unlock()

	out := make([]CounterRow, 0, len(rows))
	for _, c := range rows {
		if f.Within.IsValid() && !within(f.Within, c.Address) {
			continue
		}
		banSince, chSince := time.UnixMilli(c.BanQuietSince), time.UnixMilli(c.ChallengeQuietSince)
		r := CounterRow{Address: c.Address, Country: c.Country, ASN: c.Asn, LastSource: c.LastSource, State: StateNone,
			Ban:                    Effective(int(c.BanCount), banSince, now, d),
			Challenge:              Effective(int(c.ChallengeCount), chSince, now, d),
			NextBanDecrement:       NextDecrement(int(c.BanCount), banSince, now, d),
			NextChallengeDecrement: NextDecrement(int(c.ChallengeCount), chSince, now, d)}
		if c.LastBanAt.Valid {
			r.LastBan = time.UnixMilli(c.LastBanAt.Int64)
		}
		if c.LastChallengeAt.Valid {
			r.LastChallenge = time.UnixMilli(c.LastChallengeAt.Int64)
		}
		if st, ok := state[c.Address]; ok {
			x := snap[c.Address]
			r.State, r.Until = st, x.Expires
			if st == StateChallenged {
				r.Failures, r.MaxFailures = x.Failures, cfg.ChallengeMaxFailed
			}
		}
		if !keep(f, r) {
			continue
		}
		out = append(out, r)
	}
	slices.SortStableFunc(out, func(a, b CounterRow) int {
		switch f.Sort {
		case "ban":
			return b.Ban - a.Ban
		case "challenge":
			return b.Challenge - a.Challenge
		}
		return b.last().Compare(a.last())
	})
	return out, nil
}

func keep(f CounterFilter, r CounterRow) bool {
	switch f.State {
	case StateBanned, StateChallenged:
		if r.State != f.State {
			return false
		}
	case "active":
		if r.State == StateNone {
			return false
		}
	}
	return r.Ban >= f.MinBan && r.Challenge >= f.MinChallenge
}

// within reports whether a counter key (an address or an IPv6 /64) lies inside p.
func within(p netip.Prefix, key string) bool {
	if strings.Contains(key, "/") {
		k, err := netip.ParsePrefix(key)
		return err == nil && p.Overlaps(k) && p.Bits() <= k.Bits()
	}
	a, err := netip.ParseAddr(key)
	return err == nil && p.Contains(a)
}

// CounterEdit changes one address's counters; nil leaves a counter as it is. A value set counts from now.
type CounterEdit struct {
	Ban, Challenge *int
	ResetFailures  bool // a live challenge starts counting unsolved requests again
}

// EditCounters applies ed to a's counters (06 §5.1, §13.1).
func (e *Engine) EditCounters(ctx context.Context, a netip.Addr, ed CounterEdit, actor string) error {
	k, now := Key(a.Unmap()), e.now()
	// Memory first, then the write: the engine never holds its lock while waiting for the database.
	var failuresOf string
	// A set ban counter starts its quiet time at the later of now and the end of a running ban, as a ban itself does
	// (06 §5.1): it must not decay while that ban runs. A permanent ban never ends.
	banQuiet := now
	e.mu.Lock()
	if x, ok := e.challenged[k]; ok && ed.ResetFailures {
		x.Failures, x.dirty, failuresOf = 0, false, x.ElementID
	}
	if b, ok := e.banned[k]; ok && b.live(now) {
		switch {
		case b.Expires.IsZero():
			banQuiet = never
		case b.Expires.After(now):
			banQuiet = b.Expires
		}
	}
	e.mu.Unlock()
	err := e.st.Tx(ctx, func(q *db.Queries) error {
		if ed.Ban != nil || ed.Challenge != nil {
			c, err := counters(ctx, q, prefixKey(k))
			if err != nil {
				return err
			}
			if ed.Ban != nil {
				c.BanCount, c.BanQuietSince = int64(max(*ed.Ban, 0)), banQuiet.UnixMilli()
			}
			if ed.Challenge != nil {
				c.ChallengeCount, c.ChallengeQuietSince = int64(max(*ed.Challenge, 0)), now.UnixMilli()
			}
			if err := putCounters(ctx, q, c, now); err != nil {
				return err
			}
		}
		if failuresOf == "" {
			return nil
		}
		return q.SetDynamicEntryFailures(ctx, db.SetDynamicEntryFailuresParams{FailureCount: 0, ElementID: failuresOf})
	})
	if err != nil {
		return err
	}
	meta := map[string]any{"actor": actor, "reset_failures": ed.ResetFailures}
	if ed.Ban != nil {
		meta["ban_count"] = *ed.Ban
	}
	if ed.Challenge != nil {
		meta["challenge_count"] = *ed.Challenge
	}
	e.ev.add(event{at: now, key: k, typ: "counters_edited", severity: "info", source: SourceManual, meta: meta})
	return nil
}

// ResetCounters sets both counters of every given key to 0 and reports how many records changed.
func (e *Engine) ResetCounters(ctx context.Context, keys []string, actor string) (int, error) {
	now, n := e.now(), 0
	err := e.st.Tx(ctx, func(q *db.Queries) error {
		for _, k := range keys {
			c, err := q.GetAddressCounters(ctx, k)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					continue
				}
				return err
			}
			c.BanCount, c.BanQuietSince, c.ChallengeCount, c.ChallengeQuietSince = 0, now.UnixMilli(), 0, now.UnixMilli()
			if err := putCounters(ctx, q, c, now); err != nil {
				return err
			}
			n++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	for _, k := range keys[:min(len(keys), 50)] { // one event per address, bounded for a bulk reset
		if p, ok := parseKey(k); ok {
			e.ev.add(event{at: now, key: p, typ: "counters_reset", severity: "info", source: SourceManual, meta: map[string]any{"actor": actor}})
		}
	}
	return n, nil
}

func parseKey(k string) (netip.Prefix, bool) {
	if a, err := netip.ParseAddr(k); err == nil {
		return Key(a), true
	}
	p, err := netip.ParsePrefix(k)
	return p, err == nil
}
