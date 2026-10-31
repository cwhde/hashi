package monitor

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"

	"git.juzo.io/juzo/hashi/internal/services"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

// Strip is the 60-minute history: per-sample timestamps and verdicts, oldest first (07 §4).
type Strip []StripSample

// StripSample is one round.
type StripSample struct {
	At      time.Time
	Verdict string
}

// Row is an endpoint for lists.
type Row struct {
	Endpoint db.MonitorEndpoint
	State    string // paused replaces the stored state
	Checks   []db.MonitorCheck
	Strip    Strip
	Uptime24 *float64
}

func shownState(e db.MonitorEndpoint) string {
	if e.Paused == 1 {
		return StatePaused
	}
	return e.State
}

// List returns every endpoint with its strip and 24-hour uptime.
func (s *Service) List(ctx context.Context) ([]Row, error) {
	rows, err := s.st.R.ListMonitorEndpoints(ctx)
	if err != nil {
		return nil, err
	}
	strips, err := s.strips(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Row, 0, len(rows))
	for _, e := range rows {
		checks, err := s.st.R.ListMonitorChecks(ctx, e.ID)
		if err != nil {
			return nil, err
		}
		up, err := s.uptime(ctx, e.ID, 24*time.Hour)
		if err != nil {
			return nil, err
		}
		out = append(out, Row{Endpoint: e, State: shownState(e), Checks: checks, Strip: strips[e.ID], Uptime24: up})
	}
	return out, nil
}

func (s *Service) strips(ctx context.Context) (map[string]Strip, error) {
	rounds, err := s.st.R.ListRoundsSince(ctx, s.now().Add(-time.Hour).UnixMilli())
	if err != nil {
		return nil, err
	}
	out := map[string]Strip{}
	for _, r := range rounds {
		out[r.EndpointID] = append(out[r.EndpointID], StripSample{At: time.UnixMilli(r.At).UTC(), Verdict: r.Verdict})
	}
	return out, nil
}

// uptime is the share of rounds not down, or nil without data.
func (s *Service) uptime(ctx context.Context, id string, d time.Duration) (*float64, error) {
	r, err := s.st.R.UptimeSince(ctx, db.UptimeSinceParams{EndpointID: id, At: s.now().Add(-d).UnixMilli()})
	if err != nil {
		return nil, err
	}
	if r.Total == 0 {
		return nil, nil
	}
	v := float64(r.OkCount) / float64(r.Total) * 100
	return &v, nil
}

// Point is one chart point.
type Point struct {
	At           time.Time
	LatencyMs    int64
	Availability float64
	GapMs        *int64
}

// Detail is one endpoint's page.
type Detail struct {
	Row
	Stats          map[string]*float64 // uptime by "1h", "24h", "7d", "30d"
	MinMs, MaxMs   *int64
	AvgMs          *int64
	Series         []Point
	Events         []db.MonitorEvent
	CheckLastError map[string]string
}

// Ranges maps a chart range to its span and resolution in seconds; 1 h uses the 1-minute buckets (07 §5).
var Ranges = map[string]struct {
	Span       time.Duration
	Resolution int64
}{
	"1h":  {time.Hour, 60},
	"24h": {24 * time.Hour, 300},
	"7d":  {7 * 24 * time.Hour, 3600},
	"30d": {30 * 24 * time.Hour, 3600},
}

// Get returns an endpoint's detail for a chart range. Only closed buckets are charted.
func (s *Service) Get(ctx context.Context, id, rng string) (Detail, error) {
	e, err := s.st.R.GetMonitorEndpoint(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Detail{}, ErrNotFound
	}
	if err != nil {
		return Detail{}, err
	}
	r, ok := Ranges[rng]
	if !ok {
		r = Ranges["24h"]
	}
	checks, err := s.st.R.ListMonitorChecks(ctx, id)
	if err != nil {
		return Detail{}, err
	}
	strips, err := s.strips(ctx)
	if err != nil {
		return Detail{}, err
	}
	d := Detail{Row: Row{Endpoint: e, State: shownState(e), Checks: checks, Strip: strips[id]}, Stats: map[string]*float64{}, CheckLastError: map[string]string{}}
	for k, v := range map[string]time.Duration{"1h": time.Hour, "24h": 24 * time.Hour, "7d": 7 * 24 * time.Hour, "30d": 30 * 24 * time.Hour} {
		if d.Stats[k], err = s.uptime(ctx, id, v); err != nil {
			return Detail{}, err
		}
	}
	d.Uptime24 = d.Stats["24h"]
	since := s.now().Add(-r.Span).UnixMilli()
	byCheck := func(check string) ([]db.MonitorRollup, error) {
		return s.st.R.ListMonitorRollups(ctx, db.ListMonitorRollupsParams{EndpointID: id, CheckID: check, Resolution: r.Resolution, Bucket: since})
	}
	rounds, err := byCheck("")
	if err != nil {
		return Detail{}, err
	}
	var pub, internal map[int64]int64
	for _, c := range checks {
		if c.Role != "public" && c.Role != "internal" || c.Type == TypeTLS {
			continue
		}
		rows, err := byCheck(c.ID)
		if err != nil {
			return Detail{}, err
		}
		m := map[int64]int64{}
		for _, row := range rows {
			m[row.Bucket] = row.AvgMs
		}
		if c.Role == "public" {
			pub = m
		} else {
			internal = m
		}
	}
	var sum, n int64
	for _, row := range rounds {
		p := Point{At: time.UnixMilli(row.Bucket).UTC(), LatencyMs: row.AvgMs, Availability: float64(row.OkCount) / float64(row.Count) * 100}
		if a, ok := pub[row.Bucket]; ok {
			if b, ok := internal[row.Bucket]; ok {
				gap := a - b
				p.GapMs = &gap
			}
		}
		d.Series = append(d.Series, p)
		if d.MinMs == nil || row.MinMs < *d.MinMs {
			v := row.MinMs
			d.MinMs = &v
		}
		if d.MaxMs == nil || row.MaxMs > *d.MaxMs {
			v := row.MaxMs
			d.MaxMs = &v
		}
		sum += row.AvgMs * row.Count
		n += row.Count
	}
	if n > 0 {
		v := sum / n
		d.AvgMs = &v
	}
	if d.Events, err = s.st.R.ListMonitorEvents(ctx, db.ListMonitorEventsParams{EndpointID: id, Limit: 50}); err != nil {
		return Detail{}, err
	}
	for _, c := range checks {
		rows, err := s.st.R.ListMonitorSamplesSince(ctx, db.ListMonitorSamplesSinceParams{EndpointID: id, CheckID: c.ID, At: s.now().Add(-time.Duration(e.IntervalSeconds*3) * time.Second).UnixMilli()})
		if err != nil {
			return Detail{}, err
		}
		if len(rows) > 0 && rows[len(rows)-1].Ok == 0 {
			d.CheckLastError[c.ID] = rows[len(rows)-1].Detail
		}
	}
	return d, nil
}

// Settings are what the admin may change on an endpoint.
type Settings struct {
	Group           string
	Paused          bool
	IntervalSeconds int64
	TimeoutSeconds  int64
	DegradedMs      int64
	Public          bool
	RouteProviderID string
}

// Update stores an endpoint's settings. Resuming starts the state over: the pause says nothing about what happened.
func (s *Service) Update(ctx context.Context, id string, v Settings) error {
	v.Group = strings.TrimSpace(v.Group)
	if v.Group == "" {
		return errInvalid("a group is required")
	}
	if v.IntervalSeconds < 10 || v.IntervalSeconds > 86400 {
		return errInvalid("the interval must be between 10 seconds and a day")
	}
	if v.TimeoutSeconds < 1 || v.TimeoutSeconds > 60 {
		return errInvalid("the timeout must be between 1 and 60 seconds")
	}
	if v.DegradedMs < 0 {
		return errInvalid("the latency threshold cannot be negative")
	}
	now := s.now().UnixMilli()
	return s.st.Tx(ctx, func(q *db.Queries) error {
		e, err := q.GetMonitorEndpoint(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if e.Paused == 1 && !v.Paused {
			if err := q.SetMonitorEndpointResult(ctx, db.SetMonitorEndpointResultParams{State: StateUnknown, StateSince: now, ID: id}); err != nil {
				return err
			}
		}
		return q.UpdateMonitorEndpointSettings(ctx, db.UpdateMonitorEndpointSettingsParams{GroupName: v.Group, Paused: flag(v.Paused), IntervalSeconds: v.IntervalSeconds,
			TimeoutSeconds: v.TimeoutSeconds, DegradedMs: v.DegradedMs, Public: flag(v.Public),
			RouteProviderID: sql.NullString{String: v.RouteProviderID, Valid: v.RouteProviderID != ""}, UpdatedAt: now, ID: id})
	})
}

func errInvalid(msg string) error { return &invalidError{msg} }

type invalidError struct{ msg string }

func (e *invalidError) Error() string { return e.msg }
func (e *invalidError) Unwrap() error { return ErrInvalid }

// CreateUser adds a user monitor with one check (07 §1).
func (s *Service) CreateUser(ctx context.Context, name, group string, c CheckSpec) (string, error) {
	name, group = strings.TrimSpace(name), strings.TrimSpace(group)
	if name == "" {
		return "", errInvalid("a name is required")
	}
	if group == "" {
		group = "My checks"
	}
	if err := validateSpec(c); err != nil {
		return "", err
	}
	c.Required = true
	id := ulid.Make().String()
	now := s.now().UnixMilli()
	err := s.st.Tx(ctx, func(q *db.Queries) error {
		if err := q.InsertMonitorEndpoint(ctx, db.InsertMonitorEndpointParams{ID: id, SourceType: SourceUser, Name: name, GroupName: group, IntervalSeconds: 60, TimeoutSeconds: 10, StateSince: now, CreatedAt: now, UpdatedAt: now}); err != nil {
			return err
		}
		return insertChecks(ctx, q, id, []CheckSpec{c})
	})
	return id, err
}

func validateSpec(c CheckSpec) error {
	if strings.TrimSpace(c.Target) == "" {
		return errInvalid("a target is required")
	}
	switch c.Type {
	case TypeHTTP, TypeH2C:
		if !strings.HasPrefix(c.Target, "http://") {
			return errInvalid("this check needs an http:// address")
		}
	case TypeHTTPS:
		if !strings.HasPrefix(c.Target, "https://") {
			return errInvalid("this check needs an https:// address")
		}
	case TypeTCP, TypeTLS:
	case TypeUDP:
		if c.Probe == "" || c.Expect == "" {
			return errInvalid("a UDP check needs a probe payload and an expected response: no response is never healthy")
		}
	case TypeDNS:
	default:
		return errInvalid("unknown check type")
	}
	return nil
}

// DeleteUser removes a user monitor; sourced endpoints follow their source.
func (s *Service) DeleteUser(ctx context.Context, id string) error {
	return s.st.Tx(ctx, func(q *db.Queries) error {
		e, err := q.GetMonitorEndpoint(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if e.SourceType != SourceUser {
			return ErrSourced
		}
		return q.DeleteMonitorEndpoint(ctx, id)
	})
}

// PublicEndpoint holds only what the public status page may show (07 §7): no target, no message, no source.
type PublicEndpoint struct {
	Name     string
	Group    string
	State    string
	Strip    Strip
	Uptime24 *float64
	// PendingDNS: its address changed and DNS may still be catching up.
	PendingDNS bool
}

// PublicStatus is the status page's data.
type PublicStatus struct {
	Endpoints []PublicEndpoint
	UpdatedAt time.Time
}

// PublicApp holds only what the dashboard may show (07 §8).
type PublicApp struct {
	Name  string
	Group string
	URL   string
	State string
}

// PublicApps is the dashboard's data.
type PublicApps struct {
	Apps                      []PublicApp
	HostsOnline, HostsTotal   int
	FirewallUp, FirewallTotal int
	UpdatedAt                 time.Time
}

// cache keeps public answers for 15 s (07 §7, §8): visitors never cause database work per request.
type cache[T any] struct {
	mu  sync.Mutex
	val T
	at  time.Time
}

const publicCache = 15 * time.Second

func (c *cache[T]) get(now time.Time, load func() (T, error)) (T, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.at.IsZero() && now.Sub(c.at) < publicCache {
		return c.val, nil
	}
	v, err := load()
	if err != nil {
		return c.val, err
	}
	c.val, c.at = v, now
	return v, nil
}

// PublicStatus returns the public endpoints. UpdatedAt is the last check of any endpoint: when checks stop, the page
// says how old its data is (07 §7).
func (s *Service) PublicStatus(ctx context.Context) (PublicStatus, error) {
	return s.statusCache.get(s.now(), func() (PublicStatus, error) {
		rows, err := s.List(ctx)
		if err != nil {
			return PublicStatus{}, err
		}
		out := PublicStatus{Endpoints: []PublicEndpoint{}}
		for _, r := range rows {
			if r.Endpoint.LastCheckAt.Valid {
				if t := time.UnixMilli(r.Endpoint.LastCheckAt.Int64).UTC(); t.After(out.UpdatedAt) {
					out.UpdatedAt = t
				}
			}
			if r.Endpoint.Public == 1 {
				out.Endpoints = append(out.Endpoints, PublicEndpoint{PendingDNS: r.Endpoint.PendingDns == 1, Name: r.Endpoint.Name, Group: r.Endpoint.GroupName, State: r.State, Strip: r.Strip, Uptime24: r.Uptime24})
			}
		}
		return out, nil
	})
}

// PublicApps returns the dashboard: services with the dashboard flag and a public URL, and manual DNS entries with it.
func (s *Service) PublicApps(ctx context.Context) (PublicApps, error) {
	return s.appsCache.get(s.now(), func() (PublicApps, error) {
		rows, err := s.st.R.ListMonitorEndpoints(ctx)
		if err != nil {
			return PublicApps{}, err
		}
		bySource := map[string]db.MonitorEndpoint{}
		out := PublicApps{Apps: []PublicApp{}}
		for _, e := range rows {
			bySource[e.SourceType+"|"+e.SourceID] = e
			if e.LastCheckAt.Valid {
				if t := time.UnixMilli(e.LastCheckAt.Int64).UTC(); t.After(out.UpdatedAt) {
					out.UpdatedAt = t
				}
			}
			if e.SourceType == SourceFirewall {
				out.FirewallTotal++
				if e.State == StateUp || e.State == StateDegraded {
					out.FirewallUp++
				}
			}
		}
		add := func(name, url, source string, group string) {
			app := PublicApp{Name: name, Group: group, URL: url, State: StateUnknown}
			if e, ok := bySource[source]; ok {
				app.State, app.Group = shownState(e), e.GroupName
			}
			out.Apps = append(out.Apps, app)
			out.HostsTotal++
			if app.State == StateUp || app.State == StateDegraded {
				out.HostsOnline++
			}
		}
		svcs, err := services.List(ctx, s.st.R)
		if err != nil {
			return PublicApps{}, err
		}
		for _, v := range svcs {
			if v.Renderable() && v.DashboardVisible && v.HostName != "" {
				add(v.Name, "https://"+strings.TrimSuffix(v.HostName, ".")+"/", SourceService+"|"+v.ID, "Services")
			}
		}
		recs, err := s.st.R.ListDashboardDNSRecords(ctx)
		if err != nil {
			return PublicApps{}, err
		}
		for _, r := range recs {
			host := r.ZoneName
			if r.Name != "@" {
				host = r.Name + "." + r.ZoneName
			}
			name := r.DisplayName
			if name == "" {
				name = host
			}
			add(name, "https://"+host+"/", SourceDNS+"|"+r.ID, "DNS entries")
		}
		slices.SortStableFunc(out.Apps, func(a, b PublicApp) int { return strings.Compare(a.Name, b.Name) })
		return out, nil
	})
}
