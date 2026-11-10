package httpapi

import (
	"context"
	"net/netip"
	"strconv"
	"time"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
	"git.juzo.io/juzo/hashi/internal/security"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

// or is *p, or d when p is nil.
func or[T any](p *T, d T) T {
	if p == nil {
		return d
	}
	return *p
}

// counterFilter reads the list's filter; the reset's "all matching" passes the same fields.
func counterFilter(state *string, minBan, minChallenge *int, source *gen.SecuritySource, country *string, asn *int,
	since *time.Time, q *string) (security.CounterFilter, error) {
	f := security.CounterFilter{State: orDefault(state, "all"), MinBan: or(minBan, 0), MinChallenge: or(minChallenge, 0),
		Country: orDefault(country, ""), ASN: int64(or(asn, 0))}
	if source != nil {
		f.Source = string(*source)
	}
	if since != nil {
		f.Since = *since
	}
	if q != nil && *q != "" {
		p, err := netip.ParsePrefix(*q)
		if err != nil {
			a, aerr := netip.ParseAddr(*q)
			if aerr != nil {
				return f, invalid("q must be an address or a CIDR")
			}
			p = netip.PrefixFrom(a.Unmap(), a.Unmap().BitLen())
		}
		f.Within = p.Masked()
	}
	return f, nil
}

func counterRow(r security.CounterRow) gen.CounterRow {
	at := func(t time.Time) *time.Time {
		if t.IsZero() {
			return nil
		}
		u := t.UTC()
		return &u
	}
	return gen.CounterRow{Address: r.Address, Country: r.Country, Asn: int(r.ASN), BanCount: r.Ban, ChallengeCount: r.Challenge,
		State: gen.CounterRowState(r.State), Until: at(r.Until), Failures: r.Failures, MaxFailures: r.MaxFailures,
		LastBan: at(r.LastBan), LastChallenge: at(r.LastChallenge), NextBanDecrement: at(r.NextBanDecrement),
		NextChallengeDecrement: at(r.NextChallengeDecrement), LastSource: r.LastSource}
}

// ListSecurityCounters pages the counters view (06 §13.1); the cursor is the offset into the sorted rows.
func (s *Server) ListSecurityCounters(ctx context.Context, req gen.ListSecurityCountersRequestObject) (gen.ListSecurityCountersResponseObject, error) {
	p := req.Params
	var state *string
	if p.State != nil {
		v := string(*p.State)
		state = &v
	}
	f, err := counterFilter(state, p.MinBan, p.MinChallenge, p.Source, p.Country, p.Asn, p.Since, p.Q)
	if err != nil {
		return nil, err
	}
	if p.Sort != nil {
		f.Sort = string(*p.Sort)
	}
	rows, err := s.Security.Counters(ctx, f)
	if err != nil {
		return nil, err
	}
	off := 0
	if p.Cursor != nil {
		if off, err = strconv.Atoi(*p.Cursor); err != nil || off < 0 {
			return nil, invalid("cursor is not one this list gave")
		}
	}
	limit := or(p.Limit, 100)
	out := gen.CounterList{Rows: []gen.CounterRow{}, Total: len(rows)}
	for _, r := range rows[min(off, len(rows)):min(off+limit, len(rows))] {
		out.Rows = append(out.Rows, counterRow(r))
	}
	if off+limit < len(rows) {
		next := strconv.Itoa(off + limit)
		out.NextCursor = &next
	}
	return gen.ListSecurityCounters200JSONResponse(out), nil
}

// EditAddressCounters sets one address's counters (06 §5.1); each value decays from now.
func (s *Server) EditAddressCounters(ctx context.Context, req gen.EditAddressCountersRequestObject) (gen.EditAddressCountersResponseObject, error) {
	a, err := netip.ParseAddr(req.Address)
	if err != nil {
		return nil, invalid("not an address")
	}
	b := req.Body
	if b.BanCount == nil && b.ChallengeCount == nil && !or(b.ResetFailures, false) {
		return nil, invalid("nothing to change")
	}
	if (b.BanCount != nil && *b.BanCount < 0) || (b.ChallengeCount != nil && *b.ChallengeCount < 0) {
		return nil, invalid("counters are 0 or more")
	}
	ri := info(ctx)
	ed := security.CounterEdit{Ban: b.BanCount, Challenge: b.ChallengeCount, ResetFailures: or(b.ResetFailures, false)}
	before, err := s.Security.EditCounters(ctx, a, ed, s.actor(ri))
	if err != nil {
		return nil, err
	}
	key := security.KeyString(a)
	// Before and after (06 §16): the before values are the effective ones the counters view showed, so setting
	// them again restores the state.
	detail := map[string]any{"reset_failures": ed.ResetFailures}
	if b.BanCount != nil {
		detail["ban"] = map[string]int{"before": before.Ban, "after": *b.BanCount}
	}
	if b.ChallengeCount != nil {
		detail["challenge"] = map[string]int{"before": before.Challenge, "after": *b.ChallengeCount}
	}
	if err := s.Store.Tx(ctx, func(q *db.Queries) error {
		return s.Audit.Record(ctx, q, audit.Event{Actor: s.actor(ri), Action: "security.counters_edited", IP: ri.ipString(),
			Target: "address:" + key, Outcome: audit.Success, Detail: detail})
	}); err != nil {
		return nil, err
	}
	row, err := s.counterOf(ctx, a)
	if err != nil {
		return nil, err
	}
	return gen.EditAddressCounters200JSONResponse(row), nil
}

// ResetSecurityCounters zeroes the counters of the given addresses or of every row the filter matches.
func (s *Server) ResetSecurityCounters(ctx context.Context, req gen.ResetSecurityCountersRequestObject) (gen.ResetSecurityCountersResponseObject, error) {
	b := req.Body
	var keys []string
	switch {
	case b.Addresses != nil && b.AllMatching != nil:
		return nil, invalid("give addresses or all_matching, not both")
	case b.Addresses != nil:
		for _, v := range *b.Addresses {
			a, err := netip.ParseAddr(v)
			if err != nil {
				if p, perr := netip.ParsePrefix(v); perr == nil && p.Addr().Is6() && p.Bits() == 64 {
					keys = append(keys, p.Masked().String())
					continue
				}
				return nil, invalid(v + " is not an address")
			}
			keys = append(keys, security.KeyString(a))
		}
	case b.AllMatching != nil:
		m := b.AllMatching
		var state *string
		if m.State != nil {
			v := string(*m.State)
			state = &v
		}
		f, err := counterFilter(state, m.MinBan, m.MinChallenge, m.Source, m.Country, m.Asn, m.Since, m.Q)
		if err != nil {
			return nil, err
		}
		rows, err := s.Security.Counters(ctx, f)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			keys = append(keys, r.Address)
		}
	default:
		return nil, invalid("give addresses or all_matching")
	}
	ri := info(ctx)
	before, err := s.Security.ResetCounters(ctx, keys, s.actor(ri))
	if err != nil {
		return nil, err
	}
	n := len(before)
	// Every record that was above 0 keeps its previous values here (06 §16), so a mistaken bulk reset can be undone.
	prev := []map[string]any{}
	for _, v := range before {
		if v.Ban > 0 || v.Challenge > 0 {
			prev = append(prev, map[string]any{"address": v.Address, "ban": v.Ban, "challenge": v.Challenge})
		}
	}
	if err := s.Store.Tx(ctx, func(q *db.Queries) error {
		return s.Audit.Record(ctx, q, audit.Event{Actor: s.actor(ri), Action: "security.counters_reset", IP: ri.ipString(),
			Target: "counters", Outcome: audit.Success, Detail: map[string]any{"count": n, "by_filter": b.AllMatching != nil,
				"before": prev}})
	}); err != nil {
		return nil, err
	}
	return gen.ResetSecurityCounters200JSONResponse{Count: n}, nil
}

// counterOf is one address's counters row, or an empty one.
func (s *Server) counterOf(ctx context.Context, a netip.Addr) (gen.CounterRow, error) {
	key := security.KeyString(a)
	rows, err := s.Security.Counters(ctx, security.CounterFilter{Within: security.Key(a.Unmap())})
	if err != nil {
		return gen.CounterRow{}, err
	}
	for _, r := range rows {
		if r.Address == key {
			return counterRow(r), nil
		}
	}
	return gen.CounterRow{Address: key, State: gen.CounterRowStateNone}, nil
}

// AddDynamicEntry is "challenge now" or "ban now" (05a §8): the engine's own path, so counters, events and the
// host link follow as for any other trigger. An allowed address is never challenged or banned (invariant 20).
func (s *Server) AddDynamicEntry(ctx context.Context, req gen.AddDynamicEntryRequestObject) (gen.AddDynamicEntryResponseObject, error) {
	a, err := netip.ParseAddr(req.Body.Address)
	if err != nil {
		return nil, invalid("not an address")
	}
	ri := info(ctx)
	c := security.Cause{Source: security.SourceManual, Actor: s.actor(ri), Reason: or(req.Body.Reason, "")}
	if d := req.Body.DurationSeconds; d != nil {
		if *d < 60 {
			return nil, invalid("a duration is at least 60 seconds")
		}
		c.Duration = time.Duration(*d) * time.Second
	}
	if c.Reason == "" {
		c.Reason = "by hand"
	}
	var created bool
	switch req.Id {
	case security.SetChallenge:
		created, err = s.Security.Challenge(ctx, a, c)
	case security.SetBlocked:
		created, err = s.Security.Ban(ctx, a, c)
	default:
		return nil, invalid("entries go into " + security.SetChallenge + " or " + security.SetBlocked)
	}
	if err != nil {
		return nil, err
	}
	if err := s.Store.Tx(ctx, func(q *db.Queries) error {
		return s.Audit.Record(ctx, q, audit.Event{Actor: c.Actor, Action: "security.dynamic_added", IP: ri.ipString(),
			Target: "address:" + security.KeyString(a), Outcome: audit.Success, Detail: map[string]any{"set": req.Id, "created": created,
				"duration_seconds": req.Body.DurationSeconds}})
	}); err != nil {
		return nil, err
	}
	row, err := s.counterOf(ctx, a)
	if err != nil {
		return nil, err
	}
	return gen.AddDynamicEntry200JSONResponse{Created: created, Counters: row}, nil
}

// RemoveDynamicEntry takes an address out of both dynamic sets; its counters stay (06 §5.1).
func (s *Server) RemoveDynamicEntry(ctx context.Context, req gen.RemoveDynamicEntryRequestObject) (gen.RemoveDynamicEntryResponseObject, error) {
	if req.Id != security.SetChallenge && req.Id != security.SetBlocked {
		return nil, invalid("entries live in " + security.SetChallenge + " or " + security.SetBlocked)
	}
	a, err := netip.ParseAddr(req.Address)
	if err != nil {
		return nil, invalid("not an address")
	}
	ri := info(ctx)
	removed, err := s.Security.Remove(ctx, security.Key(a.Unmap()), s.actor(ri), "removed by hand")
	if err != nil {
		return nil, err
	}
	if removed {
		if err := s.Store.Tx(ctx, func(q *db.Queries) error {
			return s.Audit.Record(ctx, q, audit.Event{Actor: s.actor(ri), Action: "security.dynamic_removed", IP: ri.ipString(),
				Target: "address:" + security.KeyString(a), Outcome: audit.Success})
		}); err != nil {
			return nil, err
		}
	}
	return gen.RemoveDynamicEntry204Response{}, nil
}
