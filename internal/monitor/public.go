package monitor

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"git.juzo.io/juzo/hashi/internal/store/db"
)

// Public data for the status and apps pages (07 §7, §8): only what a visitor may see, in the layout's order. Never a
// target, a port, a message or an error text.

// PublicMinute is one round of the last hour.
type PublicMinute struct {
	At        time.Time
	State     string
	LatencyMs int64
}

// PublicSpell is a downtime or a slow spell. End is zero while it lasts.
type PublicSpell struct {
	State      string // down or degraded
	Start, End time.Time
}

// PublicDay is one bar of the 90-day strip, in Hashi's time zone (TZ).
type PublicDay struct {
	Date   string // 2006-01-02
	State  string // down, degraded, up or unknown (no data)
	Uptime *float64
	Spells []PublicSpell
}

// PublicEntry is one row of the status page.
type PublicEntry struct {
	Name, Slug, State  string
	Since              time.Time
	PendingDNS         bool
	Minutes            []PublicMinute
	Days               []PublicDay
	Uptime24, Uptime90 *float64
	IconType, IconID   string

	endpoint *db.MonitorEndpoint
}

// PublicStatusGroup is a group with at least one entry on the status page.
type PublicStatusGroup struct {
	Name    string
	Entries []PublicEntry
}

// PublicStatus is the status page's data.
type PublicStatus struct {
	Groups    []PublicStatusGroup
	UpdatedAt time.Time
}

// PublicApp is one card of the apps page.
type PublicApp struct {
	Name, URL, State string
	HasCheck         bool
	IconType, IconID string
}

// PublicAppGroup is a group with at least one app.
type PublicAppGroup struct {
	Name string
	Apps []PublicApp
}

// PublicApps is the apps page's data.
type PublicApps struct {
	Groups    []PublicAppGroup
	UpdatedAt time.Time
}

const publicDays = 90

// history is what the public pages show of one endpoint's past.
type history struct {
	minutes            []PublicMinute
	days               []PublicDay
	spells             []PublicSpell
	uptime24, uptime90 *float64
}

func share(ok, total int64) *float64 {
	if total == 0 {
		return nil
	}
	v := float64(ok) / float64(total) * 100
	return &v
}

func bad(state string) bool { return state == StateDown || state == StateDegraded }

// publicHistory reads the last hour's rounds, 90 days of hourly rollups and the state changes for the given endpoints.
// Days follow Hashi's time zone; uptime counts slow rounds as up (07 §7.1).
func (s *Service) publicHistory(ctx context.Context, q *db.Queries, eps map[string]*db.MonitorEndpoint) (map[string]*history, error) {
	now := s.now()
	y, m, d := now.In(time.Local).Date()
	first := time.Date(y, m, d-(publicDays-1), 0, 0, 0, 0, time.Local)
	out := map[string]*history{}
	var one sql.NullString // a service page reads its own endpoint only; the status page reads them all
	for id := range eps {
		out[id] = &history{minutes: []PublicMinute{}}
		one = sql.NullString{String: id, Valid: len(eps) == 1}
	}
	rounds, err := q.ListRoundSamplesSince(ctx, db.ListRoundSamplesSinceParams{Endpoint: one, Since: now.Add(-time.Hour).UnixMilli()})
	if err != nil {
		return nil, fmt.Errorf("read rounds: %w", err)
	}
	for _, r := range rounds {
		if h := out[r.EndpointID]; h != nil {
			h.minutes = append(h.minutes, PublicMinute{At: time.UnixMilli(r.At).UTC(), State: r.Verdict, LatencyMs: r.LatencyMs})
		}
	}
	type tally struct{ ok, total int64 }
	perDay := map[string]*[publicDays]tally{}
	day24, all90 := map[string]*tally{}, map[string]*tally{}
	rollups, err := q.ListHourlyRoundRollupsSince(ctx, db.ListHourlyRoundRollupsSinceParams{Endpoint: one, Since: first.UnixMilli()})
	if err != nil {
		return nil, fmt.Errorf("read rollups: %w", err)
	}
	since24 := now.Add(-24 * time.Hour).UnixMilli()
	for _, r := range rollups {
		if out[r.EndpointID] == nil {
			continue
		}
		at := time.UnixMilli(r.Bucket).In(time.Local)
		i := int(time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, time.Local).Sub(first).Hours()/24 + 0.5)
		if i < 0 || i >= publicDays {
			continue
		}
		if perDay[r.EndpointID] == nil {
			perDay[r.EndpointID] = &[publicDays]tally{}
			day24[r.EndpointID], all90[r.EndpointID] = &tally{}, &tally{}
		}
		perDay[r.EndpointID][i].ok += r.OkCount
		perDay[r.EndpointID][i].total += r.Count
		all90[r.EndpointID].ok += r.OkCount
		all90[r.EndpointID].total += r.Count
		if r.Bucket >= since24 {
			day24[r.EndpointID].ok += r.OkCount
			day24[r.EndpointID].total += r.Count
		}
	}
	events, err := q.ListMonitorEventsSince(ctx, first.UnixMilli())
	if err != nil {
		return nil, fmt.Errorf("read state changes: %w", err)
	}
	byEP := map[string][]db.ListMonitorEventsSinceRow{}
	for _, e := range events {
		byEP[e.EndpointID] = append(byEP[e.EndpointID], e)
	}
	for id, h := range out {
		h.spells = spells(byEP[id], eps[id].State, first)
		if t := all90[id]; t != nil {
			h.uptime90, h.uptime24 = share(t.ok, t.total), share(day24[id].ok, day24[id].total)
		}
		h.days = make([]PublicDay, publicDays)
		for i := range publicDays {
			start := first.AddDate(0, 0, i)
			end := start.AddDate(0, 0, 1)
			day := PublicDay{Date: start.Format(time.DateOnly), State: "unknown", Spells: []PublicSpell{}}
			if t := perDay[id]; t != nil && t[i].total > 0 {
				day.State, day.Uptime = StateUp, share(t[i].ok, t[i].total)
			}
			for _, sp := range h.spells {
				stop := sp.End
				if stop.IsZero() {
					stop = now
				}
				if sp.Start.Before(end) && stop.After(start) {
					day.Spells = append(day.Spells, sp)
					if sp.State == StateDown || day.State != StateDown {
						day.State = sp.State
					}
				}
			}
			h.days[i] = day
		}
	}
	return out, nil
}

// spells turns state changes into downtimes and slow spells. Without a change in the window the current state holds
// for all of it; the first change's from-state is the state the window opened in.
func spells(events []db.ListMonitorEventsSinceRow, current string, from time.Time) []PublicSpell {
	state := current
	if len(events) > 0 {
		state = events[0].FromState
	}
	var out []PublicSpell
	open := from
	for _, e := range events {
		at := time.UnixMilli(e.At).UTC()
		if e.ToState == state {
			continue
		}
		if bad(state) {
			out = append(out, PublicSpell{State: state, Start: open.UTC(), End: at})
		}
		state, open = e.ToState, at
	}
	if bad(state) {
		out = append(out, PublicSpell{State: state, Start: open.UTC()})
	}
	if out == nil {
		out = []PublicSpell{}
	}
	return out
}

// PublicStatus returns the status page: the entries with Status on, in the layout's groups and order. UpdatedAt is
// the last check of anything shown (bug 80).
func (s *Service) PublicStatus(ctx context.Context) (PublicStatus, error) {
	return s.statusCache.get(s.now(), func() (PublicStatus, error) {
		groups, err := s.publicEntries(ctx, s.st.R)
		if err != nil {
			return PublicStatus{}, err
		}
		out := PublicStatus{Groups: groups}
		for _, g := range groups {
			for _, e := range g.Entries {
				if ep := e.endpoint; ep.LastCheckAt.Valid {
					if t := time.UnixMilli(ep.LastCheckAt.Int64).UTC(); t.After(out.UpdatedAt) {
						out.UpdatedAt = t
					}
				}
			}
		}
		return out, nil
	})
}

func (s *Service) publicEntries(ctx context.Context, q *db.Queries) ([]PublicStatusGroup, error) {
	l, err := readLayout(ctx, q)
	if err != nil {
		return nil, err
	}
	eps := map[string]*db.MonitorEndpoint{}
	for _, g := range l.Groups {
		for _, e := range g.Entries {
			if e.Status && e.endpoint != nil {
				eps[e.endpoint.ID] = e.endpoint
			}
		}
	}
	hist, err := s.publicHistory(ctx, q, eps)
	if err != nil {
		return nil, err
	}
	out := []PublicStatusGroup{}
	for _, g := range l.Groups {
		pg := PublicStatusGroup{Name: g.Name}
		for _, e := range g.Entries {
			if !e.Status || e.endpoint == nil {
				continue
			}
			ep, h := e.endpoint, hist[e.endpoint.ID]
			pg.Entries = append(pg.Entries, PublicEntry{Name: e.Name, Slug: e.Slug, State: shownState(*ep),
				Since: time.UnixMilli(ep.StateSince).UTC(), PendingDNS: ep.PendingDns == 1, Minutes: h.minutes,
				Days: h.days, Uptime24: h.uptime24, Uptime90: h.uptime90, IconType: e.IconType, IconID: e.IconID, endpoint: ep})
		}
		if len(pg.Entries) > 0 {
			out = append(out, pg)
		}
	}
	return out, nil
}

// PublicApps returns the apps page: the entries with Apps on, in the layout's groups and order.
func (s *Service) PublicApps(ctx context.Context) (PublicApps, error) {
	return s.appsCache.get(s.now(), func() (PublicApps, error) {
		l, err := readLayout(ctx, s.st.R)
		if err != nil {
			return PublicApps{}, err
		}
		out := PublicApps{Groups: []PublicAppGroup{}}
		for _, g := range l.Groups {
			pg := PublicAppGroup{Name: g.Name}
			for _, e := range g.Entries {
				if !e.Apps || !e.CanApps {
					continue
				}
				app := PublicApp{Name: e.Name, URL: e.URL, State: StateUnknown, IconType: e.IconType, IconID: e.IconID}
				if ep := e.endpoint; ep != nil {
					app.State, app.HasCheck = shownState(*ep), true
					if ep.LastCheckAt.Valid {
						if t := time.UnixMilli(ep.LastCheckAt.Int64).UTC(); t.After(out.UpdatedAt) {
							out.UpdatedAt = t
						}
					}
				}
				pg.Apps = append(pg.Apps, app)
			}
			if len(pg.Apps) > 0 {
				out.Groups = append(out.Groups, pg)
			}
		}
		return out, nil
	})
}

// PublicPoint is one point of the service page's charts.
type PublicPoint struct {
	At           time.Time
	AvgMs, P95Ms int64
	Availability float64
}

// PublicService is one service page (07 §7.2).
type PublicService struct {
	PublicEntry
	Group             string
	Uptime7, Uptime30 *float64
	AvgMs24           *int64
	Series            []PublicPoint
	Downtimes         []PublicSpell // the last 10 within 90 days, newest first
	CertDays          *int          // days left on the TLS check's certificate; nil without a TLS check
}

// ErrNoPublicService: no entry with Status on has this slug (07 §7.2, 404).
var ErrNoPublicService = errors.New("no such service on the status page")

// PublicService returns one service page for a chart range (24h, 7d or 30d), cached like the pages (07 §7.2). The entry
// is found in the cached status page, so an unknown address costs no database work.
func (s *Service) PublicService(ctx context.Context, slug, rng string) (PublicService, error) {
	status, err := s.PublicStatus(ctx)
	if err != nil {
		return PublicService{}, err
	}
	var out PublicService
	for _, g := range status.Groups {
		for _, e := range g.Entries {
			if e.Slug == slug {
				out.PublicEntry, out.Group = e, g.Name
			}
		}
	}
	if out.endpoint == nil {
		return PublicService{}, ErrNoPublicService
	}
	if rng != "7d" && rng != "30d" {
		rng = "24h"
	}
	return s.pageCache.of(slug+"|"+rng).get(s.now(), func() (PublicService, error) { return s.publicService(ctx, out, rng) })
}

func (s *Service) publicService(ctx context.Context, out PublicService, rng string) (PublicService, error) {
	ep := out.endpoint
	r := Ranges[rng]
	now := s.now()
	series, err := s.st.R.ListMonitorRollups(ctx, db.ListMonitorRollupsParams{EndpointID: ep.ID, CheckID: "", Resolution: r.Resolution,
		Bucket: now.Add(-r.Span).UnixMilli()})
	if err != nil {
		return PublicService{}, err
	}
	out.Series = make([]PublicPoint, 0, len(series))
	for _, p := range series {
		a := 0.0
		if v := share(p.OkCount, p.Count); v != nil {
			a = *v
		}
		out.Series = append(out.Series, PublicPoint{At: time.UnixMilli(p.Bucket).UTC(), AvgMs: p.AvgMs, P95Ms: p.P95Ms, Availability: a})
	}
	hourly, err := s.st.R.ListMonitorRollups(ctx, db.ListMonitorRollupsParams{EndpointID: ep.ID, CheckID: "", Resolution: 3600,
		Bucket: now.Add(-30 * 24 * time.Hour).UnixMilli()})
	if err != nil {
		return PublicService{}, err
	}
	var ok7, n7, ok30, n30, ms24, n24 int64
	for _, p := range hourly {
		ok30, n30 = ok30+p.OkCount, n30+p.Count
		if p.Bucket >= now.Add(-7*24*time.Hour).UnixMilli() {
			ok7, n7 = ok7+p.OkCount, n7+p.Count
		}
		if p.Bucket >= now.Add(-24*time.Hour).UnixMilli() {
			ms24, n24 = ms24+p.AvgMs*p.Count, n24+p.Count
		}
	}
	out.Uptime7, out.Uptime30 = share(ok7, n7), share(ok30, n30)
	if n24 > 0 {
		v := ms24 / n24
		out.AvgMs24 = &v
	}
	all, err := s.publicHistory(ctx, s.st.R, map[string]*db.MonitorEndpoint{ep.ID: ep})
	if err != nil {
		return PublicService{}, err
	}
	out.Downtimes = []PublicSpell{}
	for i := len(all[ep.ID].spells) - 1; i >= 0 && len(out.Downtimes) < 10; i-- {
		if sp := all[ep.ID].spells[i]; sp.State == StateDown {
			out.Downtimes = append(out.Downtimes, sp)
		}
	}
	checks, err := s.st.R.ListMonitorChecks(ctx, ep.ID)
	if err != nil {
		return PublicService{}, err
	}
	for _, c := range checks {
		if c.Type != TypeTLS {
			continue
		}
		detail, err := s.st.R.LatestCheckSampleDetail(ctx, db.LatestCheckSampleDetailParams{EndpointID: ep.ID, CheckID: c.ID})
		if err != nil {
			continue // no sample yet
		}
		var days int
		if _, err := fmt.Sscanf(detail, "certificate expires in %d days", &days); err == nil {
			out.CertDays = &days
		}
	}
	return out, nil
}
