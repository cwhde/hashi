package httpapi

import (
	"context"
	"encoding/json"
	"net/netip"
	"time"

	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
	"git.juzo.io/juzo/hashi/internal/security"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

// dashboardRanges maps a period to the width of one series point: 60, 48, 42 and 30 points.
var dashboardRanges = map[string]struct{ span, step time.Duration }{
	"1h":  {time.Hour, time.Minute},
	"24h": {24 * time.Hour, 30 * time.Minute},
	"7d":  {7 * 24 * time.Hour, 4 * time.Hour},
	"30d": {30 * 24 * time.Hour, 24 * time.Hour},
}

// GetSecurityDashboard aggregates the closed minutes, the events and the live dynamic sets (06 §13).
func (s *Server) GetSecurityDashboard(ctx context.Context, req gen.GetSecurityDashboardRequestObject) (gen.GetSecurityDashboardResponseObject, error) {
	rng := "24h"
	if req.Params.Range != nil {
		rng = string(*req.Params.Range)
	}
	r, ok := dashboardRanges[rng]
	if !ok {
		return nil, invalid("range must be 1h, 24h, 7d or 30d")
	}
	now := time.Now()
	step := r.step.Milliseconds()
	since := now.Add(-r.span).UnixMilli() / step * step
	q := s.Store.R
	out := gen.SecurityDashboard{Range: rng, StepSeconds: int(r.step / time.Second), Series: []gen.SecurityPoint{},
		TopAddresses: []gen.SecurityTopAddress{}, Countries: []gen.SecurityCountry{}, Services: []gen.SecurityService{},
		Meters: []gen.SecurityMeters{}, Events: []gen.SecurityEvent{}, StaleFeeds: []gen.SecurityStaleFeed{}}

	// One point per step, empty steps included, so the sparklines keep their time axis.
	points := map[int64]*gen.SecurityPoint{}
	for at := since; at <= now.UnixMilli(); at += step {
		t := time.UnixMilli(at).UTC()
		out.Series = append(out.Series, gen.SecurityPoint{At: &t})
	}
	for i := range out.Series {
		points[out.Series[i].At.UnixMilli()] = &out.Series[i]
	}
	add := func(at int64, f func(p *gen.SecurityPoint)) {
		f(&out.Totals)
		if p := points[at]; p != nil {
			f(p)
		}
	}
	decisions, err := q.SecurityDecisionSeries(ctx, db.SecurityDecisionSeriesParams{Step: step, Since: since})
	if err != nil {
		return nil, err
	}
	for _, d := range decisions {
		n := int(d.Requests)
		add(d.At, func(p *gen.SecurityPoint) {
			switch d.Decision {
			case security.DecisionAllow:
				p.Allowed += n
			case security.DecisionAuth:
				p.AuthRequired += n
			case security.DecisionChallenge:
				p.Challenged += n
			case security.DecisionDeny, security.DecisionBlocked:
				p.Denied += n
			}
		})
	}
	evs, err := q.SecurityEventSeries(ctx, db.SecurityEventSeriesParams{Step: step, Since: since})
	if err != nil {
		return nil, err
	}
	for _, e := range evs {
		n := int(e.Events)
		switch e.EventType {
		case "waf_match":
			add(e.At, func(p *gen.SecurityPoint) { p.WafMatches += n })
		case "ban_created":
			add(e.At, func(p *gen.SecurityPoint) { p.Bans += n })
			if e.Source == security.SourceChallengeFailed {
				out.Funnel.Banned += n
			}
		case "challenge_entered":
			out.Funnel.Challenged += n
		case "challenge_solved":
			out.Funnel.Solved += n
		}
	}

	if err := s.dashboardAddresses(ctx, since, &out); err != nil {
		return nil, err
	}
	countries, err := q.SecurityRefusedByCountry(ctx, since)
	if err != nil {
		return nil, err
	}
	for _, c := range countries {
		out.Countries = append(out.Countries, gen.SecurityCountry{Country: c.Country, Refused: int(c.Refused)})
	}
	svcs, err := q.SecurityRefusedByService(ctx, since)
	if err != nil {
		return nil, err
	}
	for _, v := range svcs {
		out.Services = append(out.Services, gen.SecurityService{ServiceId: v.ServiceID, Name: v.Name, Refused: int(v.Refused)})
	}
	if err := s.dashboardMeters(ctx, since, &out); err != nil {
		return nil, err
	}
	recent, err := q.ListRecentSecurityEvents(ctx, since)
	if err != nil {
		return nil, err
	}
	for _, e := range recent {
		var meta struct {
			Rules []int `json:"rules"`
		}
		_ = json.Unmarshal([]byte(e.MetadataJson), &meta) // written by the engine; a bad row only loses its rules
		if meta.Rules == nil {
			meta.Rules = []int{}
		}
		out.Events = append(out.Events, gen.SecurityEvent{At: time.UnixMilli(e.OccurredAt).UTC(), Address: e.Address, Type: e.EventType,
			Severity: gen.SecurityEventSeverity(e.Severity), Source: e.Source, Reason: e.Reason, Method: e.RequestMethod,
			Path: e.RequestPath, ServiceId: e.ServiceID, Service: e.Service, Host: e.Host, Suppressed: int(e.Suppressed), Rules: meta.Rules})
	}
	if err := s.dashboardStaleFeeds(ctx, now, &out); err != nil {
		return nil, err
	}
	return gen.GetSecurityDashboard200JSONResponse(out), nil
}

// dashboardAddresses lists the most refused addresses with where they stand now.
func (s *Server) dashboardAddresses(ctx context.Context, since int64, out *gen.SecurityDashboard) error {
	top, err := s.Store.R.SecurityTopRefused(ctx, since)
	if err != nil {
		return err
	}
	live := map[string]security.Entry{}
	state := map[string]gen.SecurityTopAddressState{}
	if s.Security != nil {
		for _, x := range s.Security.Challenges() {
			k := security.KeyString(x.Prefix.Addr())
			live[k], state[k] = x, gen.SecurityTopAddressStateChallenged
		}
		for _, x := range s.Security.Bans() { // a ban outranks a challenge
			k := security.KeyString(x.Prefix.Addr())
			live[k], state[k] = x, gen.SecurityTopAddressStateBanned
		}
	}
	for _, t := range top {
		row := gen.SecurityTopAddress{Address: t.Address, Requests: int(t.Requests), Refused: int(t.Denied + t.Challenged), State: gen.SecurityTopAddressStateNone}
		if a, ok := keyAddr(t.Address); ok && s.Security != nil {
			c, asn := s.Security.Locate(a)
			row.Country, row.Asn = c, int(asn)
		}
		if st, ok := state[t.Address]; ok {
			x := live[t.Address]
			row.State, row.Reason = st, &x.Reason
			if !x.Expires.IsZero() {
				until := x.Expires.UTC()
				row.Until = &until
			}
		}
		out.TopAddresses = append(out.TopAddresses, row)
	}
	return nil
}

// keyAddr reads a counter key: an address, or an IPv6 /64.
func keyAddr(k string) (netip.Addr, bool) {
	if a, err := netip.ParseAddr(k); err == nil {
		return a, true
	}
	p, err := netip.ParsePrefix(k)
	return p.Addr(), err == nil
}

// dashboardMeters counts kernel-meter bans per firewall host.
func (s *Server) dashboardMeters(ctx context.Context, since int64, out *gen.SecurityDashboard) error {
	rows, err := s.Store.R.SecurityMeterBans(ctx, since)
	if err != nil {
		return err
	}
	for _, r := range rows {
		out.Meters = append(out.Meters, gen.SecurityMeters{HostId: r.HostID, Host: r.Host, Bans: int(r.Bans)})
	}
	return nil
}

// dashboardStaleFeeds lists feeds without a successful fetch for two refresh intervals: a warning well before the
// feed is emptied at the stale limit (06 §8).
func (s *Server) dashboardStaleFeeds(ctx context.Context, now time.Time, out *gen.SecurityDashboard) error {
	cfg, err := settings.Get[settings.Engine](ctx, s.Store.R)
	if err != nil {
		return err
	}
	feeds, err := s.Store.R.ListFeeds(ctx)
	if err != nil {
		return err
	}
	limit := now.Add(-2 * time.Duration(cfg.FeedRefreshSeconds) * time.Second).UnixMilli()
	for _, f := range feeds {
		if f.LastSuccessAt.Valid && f.LastSuccessAt.Int64 >= limit {
			continue
		}
		set, err := s.Store.R.GetIPSet(ctx, f.IpSetID)
		if err != nil {
			return err
		}
		row := gen.SecurityStaleFeed{SetId: f.IpSetID, Name: set.Name}
		if f.LastSuccessAt.Valid {
			last := time.UnixMilli(f.LastSuccessAt.Int64).UTC()
			row.LastSuccess = &last
		}
		out.StaleFeeds = append(out.StaleFeeds, row)
	}
	return nil
}
