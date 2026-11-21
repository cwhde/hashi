package httpapi

import (
	"cmp"
	"context"
	"encoding/json"
	"net/netip"
	"slices"
	"time"

	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
	"git.juzo.io/juzo/hashi/internal/security"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/store/db"
	"golang.org/x/sync/errgroup"
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
	sp, err := s.dashboardSplit(ctx, rng, since, now)
	if err != nil {
		return nil, err
	}
	// The parts fill separate fields and read separately; they run side by side on the read pool (the handler read them
	// one after another before, and the page waited for the sum).
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		ctx := gctx
		decisions, err := q.SecurityDecisionSeries(ctx, db.SecurityDecisionSeriesParams{Step: step, Since: sp.minutes})
		if err != nil {
			return err
		}
		if sp.hourly() {
			hours, err := q.SecurityHourDecisionSeries(ctx, db.SecurityHourDecisionSeriesParams{Step: step, Since: since, Until: sp.until})
			if err != nil {
				return err
			}
			for _, h := range hours {
				decisions = append(decisions, db.SecurityDecisionSeriesRow(h))
			}
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
			return err
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
		return nil
	})
	g.Go(func() error { return s.dashboardAddresses(gctx, since, sp, &out) })
	g.Go(func() error { return s.dashboardCountries(gctx, since, sp, &out) })
	g.Go(func() error { return s.dashboardServices(gctx, since, sp, &out) })
	g.Go(func() error { return s.dashboardMeters(gctx, since, &out) })
	g.Go(func() error {
		recent, err := q.ListRecentSecurityEvents(gctx, since)
		if err != nil {
			return err
		}
		for _, e := range recent {
			out.Events = append(out.Events, s.securityEvent(e))
		}
		return nil
	})
	g.Go(func() error { return s.dashboardStaleFeeds(gctx, now, &out) })
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return gen.GetSecurityDashboard200JSONResponse(out), nil
}

// split says which part of a period comes from the hourly tables: [since, until) from hours, minutes from minutes on.
// Short periods read minutes only (until = since).
type split struct{ since, until, minutes int64 }

func (p split) hourly() bool { return p.until > p.since }

// hourlyRanges are the periods long enough to read the rolled hours (bug 99); their steps are whole hours.
var hourlyRanges = map[string]bool{"7d": true, "30d": true}

func (s *Server) dashboardSplit(ctx context.Context, rng string, since int64, now time.Time) (split, error) {
	p := split{since, since, since}
	if !hourlyRanges[rng] {
		return p, nil
	}
	rolled, err := security.RolledUntil(ctx, s.Store.R)
	if err != nil {
		return p, err
	}
	if rolled > since {
		p.until = min(rolled, now.UnixMilli())
		p.minutes = p.until
	}
	return p, nil
}

// dashboardCountries adds up refusals per country, hours and minutes, and keeps the top 8.
func (s *Server) dashboardCountries(ctx context.Context, since int64, sp split, out *gen.SecurityDashboard) error {
	q := s.Store.R
	countries, err := q.SecurityRefusedByCountry(ctx, sp.minutes)
	if err != nil {
		return err
	}
	if sp.hourly() {
		hc, err := q.SecurityHourRefusedByCountry(ctx, db.SecurityHourRefusedByCountryParams{Since: since, Until: sp.until})
		if err != nil {
			return err
		}
		for _, c := range hc {
			countries = append(countries, db.SecurityRefusedByCountryRow(c))
		}
	}
	byCountry := map[string]int{}
	for _, c := range countries {
		if _, ok := byCountry[c.Country]; !ok {
			out.Countries = append(out.Countries, gen.SecurityCountry{Country: c.Country})
		}
		byCountry[c.Country] += int(c.Refused)
	}
	for i := range out.Countries {
		out.Countries[i].Refused = byCountry[out.Countries[i].Country]
	}
	slices.SortStableFunc(out.Countries, func(a, b gen.SecurityCountry) int { return cmp.Compare(b.Refused, a.Refused) })
	out.Countries = out.Countries[:min(len(out.Countries), 8)]
	return nil
}

// dashboardServices adds up refusals per service, hours and minutes, and keeps the top 8.
func (s *Server) dashboardServices(ctx context.Context, since int64, sp split, out *gen.SecurityDashboard) error {
	q := s.Store.R
	svcs, err := q.SecurityRefusedByService(ctx, sp.minutes)
	if err != nil {
		return err
	}
	if sp.hourly() {
		hs, err := q.SecurityHourRefusedByService(ctx, db.SecurityHourRefusedByServiceParams{Since: since, Until: sp.until})
		if err != nil {
			return err
		}
		for _, v := range hs {
			svcs = append(svcs, db.SecurityRefusedByServiceRow(v))
		}
	}
	bySvc := map[string]int{}
	for _, v := range svcs {
		if _, ok := bySvc[v.ServiceID]; !ok {
			out.Services = append(out.Services, gen.SecurityService{ServiceId: v.ServiceID, Name: v.Name})
		}
		bySvc[v.ServiceID] += int(v.Refused)
	}
	for i := range out.Services {
		out.Services[i].Refused = bySvc[out.Services[i].ServiceId]
	}
	slices.SortStableFunc(out.Services, func(a, b gen.SecurityService) int { return cmp.Compare(b.Refused, a.Refused) })
	out.Services = out.Services[:min(len(out.Services), 8)]
	return nil
}

// dashboardAddresses lists the most refused addresses with where they stand now.
func (s *Server) dashboardAddresses(ctx context.Context, since int64, sp split, out *gen.SecurityDashboard) error {
	top, err := s.Store.R.SecurityTopRefused(ctx, sp.minutes)
	if err != nil {
		return err
	}
	if sp.hourly() {
		hours, err := s.Store.R.SecurityHourTopRefused(ctx, db.SecurityHourTopRefusedParams{Since: since, Until: sp.until})
		if err != nil {
			return err
		}
		for _, h := range hours {
			top = append(top, db.SecurityTopRefusedRow(h))
		}
	}
	top = mergeTop(top)
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

// mergeTop adds up the rows of one address and keeps the 10 most refused (then most requests).
func mergeTop(rows []db.SecurityTopRefusedRow) []db.SecurityTopRefusedRow {
	at := map[string]int{}
	var out []db.SecurityTopRefusedRow
	for _, r := range rows {
		i, ok := at[r.Address]
		if !ok {
			at[r.Address] = len(out)
			out = append(out, r)
			continue
		}
		out[i].Requests += r.Requests
		out[i].Denied += r.Denied
		out[i].Challenged += r.Challenged
	}
	slices.SortStableFunc(out, func(a, b db.SecurityTopRefusedRow) int {
		if c := cmp.Compare(b.Denied+b.Challenged, a.Denied+a.Challenged); c != 0 {
			return c
		}
		return cmp.Compare(b.Requests, a.Requests)
	})
	return out[:min(len(out), 10)]
}

// securityEvent is one event row as the dashboard and the address page show it, with the address's country for the
// flag (bug 120).
func (s *Server) securityEvent(e db.ListRecentSecurityEventsRow) gen.SecurityEvent {
	var meta struct {
		Rules []int `json:"rules"`
	}
	_ = json.Unmarshal([]byte(e.MetadataJson), &meta) // written by the engine; a bad row only loses its rules
	if meta.Rules == nil {
		meta.Rules = []int{}
	}
	ev := gen.SecurityEvent{At: time.UnixMilli(e.OccurredAt).UTC(), Address: e.Address, Type: e.EventType,
		Severity: gen.SecurityEventSeverity(e.Severity), Source: e.Source, Reason: e.Reason, Method: e.RequestMethod,
		Path: e.RequestPath, ServiceId: e.ServiceID, Service: e.Service, Host: e.Host, Suppressed: int(e.Suppressed), Rules: meta.Rules}
	if a, ok := keyAddr(e.Address); ok && s.Security != nil {
		if c, _ := s.Security.Locate(a); c != "" {
			ev.Country = &c
		}
	}
	return ev
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
