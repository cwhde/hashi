package monitor

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"

	"git.juzo.io/juzo/hashi/internal/services"
	"git.juzo.io/juzo/hashi/internal/store"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

// Job kinds and intervals (invariant 15).
const (
	JobChecks   = "monitor.checks"
	JobRollup   = "monitor.rollup"
	JobRetain   = "monitor.retention"
	CheckTick   = 10 * time.Second
	RollupTick  = time.Minute
	RetainTick  = time.Hour
	parallelMax = 8
)

// Source types of endpoints.
const (
	SourceService  = "service"
	SourceDNS      = "dns_record"
	SourceFirewall = "firewall_host"
	SourceTraefik  = "traefik"
	SourceHashi    = "hashi"
	SourceUser     = "user"
)

// Errors the API maps.
var (
	ErrNotFound = errors.New("endpoint not found")
	ErrInvalid  = errors.New("invalid")
	ErrSourced  = errors.New("this endpoint follows what it watches: remove or change that instead")
)

// Notifier receives state changes, in the transaction that records them.
type Notifier interface {
	StateChanged(ctx context.Context, q *db.Queries, e db.MonitorEndpoint, from, to, message string) error
}

// Service runs monitoring.
type Service struct {
	st       *store.Store
	exec     Executor
	resolve  services.TargetResolver
	notifier Notifier
	now      func() time.Time

	statusCache cache[PublicStatus]
	appsCache   cache[PublicApps]
}

// New returns the service. self answers Hashi's own endpoint.
func New(st *store.Store, self func(context.Context) error, resolve services.TargetResolver, n Notifier) *Service {
	return &Service{st: st, exec: Executor{Self: self}, resolve: resolve, notifier: n, now: time.Now}
}

// CheckSpec is what a source wants checked.
type CheckSpec struct {
	Type, Role, Target string
	Required           bool
	SkipVerify         bool
	Probe, Expect      string
	CodeMin, CodeMax   int64
}

// Desired is an endpoint a source wants.
type Desired struct {
	SourceType, SourceID, Name, Group string
	Checks                            []CheckSpec
	// Keep leaves an existing endpoint untouched: its source could not be read this time.
	Keep bool
}

// Sources derives the endpoints from what Hashi manages (07 §1). AdGuard and Pulse sources arrive with S8.
func (s *Service) Sources(ctx context.Context) ([]Desired, error) {
	out := []Desired{{SourceType: SourceHashi, Name: "Hashi", Group: "Hashi", Checks: []CheckSpec{{Type: TypeSelf, Target: "self", Required: true}}}}
	svcs, err := services.List(ctx, s.st.R)
	if err != nil {
		return nil, err
	}
	for _, v := range svcs {
		if !v.Renderable() || !v.MonitoringVisible {
			continue
		}
		if s.resolve != nil {
			// One service's address set failing must not stop the others' checks: its endpoint stays as it is.
			one, err := services.Resolve(ctx, []services.Service{v}, s.resolve)
			if err != nil {
				out = append(out, Desired{SourceType: SourceService, SourceID: v.ID, Name: v.Name, Keep: true})
				continue
			}
			if v = one[0]; !v.Renderable() {
				continue
			}
		}
		out = append(out, Desired{SourceType: SourceService, SourceID: v.ID, Name: v.Name, Group: "Services", Checks: ServiceChecks(v)})
	}
	recs, err := s.st.R.ListMonitoredDNSRecords(ctx)
	if err != nil {
		return nil, err
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
		out = append(out, Desired{SourceType: SourceDNS, SourceID: r.ID, Name: name, Group: "DNS entries", Checks: []CheckSpec{
			{Type: TypeHTTPS, Role: "public", Target: "https://" + host + "/", Required: true},
			{Type: TypeTLS, Target: net.JoinHostPort(host, "443")},
		}})
	}
	hosts, err := s.st.R.ListFirewallHosts(ctx)
	if err != nil {
		return nil, err
	}
	for _, h := range hosts {
		out = append(out, Desired{SourceType: SourceFirewall, SourceID: h.ID, Name: h.Name, Group: "Firewall hosts", Checks: []CheckSpec{
			{Type: TypeTCP, Target: net.JoinHostPort(h.SshHost, strconv.FormatInt(h.SshPort, 10)), Required: true},
		}})
	}
	conns, err := s.st.R.ListTraefikConnections(ctx)
	if err != nil {
		return nil, err
	}
	for _, c := range conns {
		if !c.OwnershipConfirmedAt.Valid {
			continue // a connection still in setup, or one that was given up on
		}
		out = append(out, Desired{SourceType: SourceTraefik, SourceID: c.ID, Name: c.Name, Group: "Traefik hosts", Checks: []CheckSpec{
			{Type: TypeTCP, Target: net.JoinHostPort(c.SshHost, "443"), Required: true},
		}})
	}
	return out, nil
}

// ServiceChecks gives a service's checks by its protocol (07 §2): the HTTP family checks the internal upstream and,
// with a name, the public URL; both paths are optional on their own, so one failing path is degraded and tells "service
// down" from "edge down". A TCP stream connects to its upstream. A UDP stream has no check until the admin configures a
// probe (a silent port is never healthy).
func ServiceChecks(v services.Service) []CheckSpec {
	var out []CheckSpec
	first := v.Target
	if len(v.Targets) > 0 {
		first = v.Targets[0]
	}
	hostport := net.JoinHostPort(first.Address, strconv.Itoa(first.Port))
	switch v.Kind {
	case services.KindTCP:
		return []CheckSpec{{Type: TypeTCP, Role: "internal", Target: hostport, Required: true}}
	case services.KindUDP:
		return nil
	case services.KindHTTPS:
		out = append(out, CheckSpec{Type: TypeHTTPS, Role: "internal", Target: "https://" + hostport + "/", SkipVerify: v.TLSSkipVerify})
	case services.KindH2C:
		out = append(out, CheckSpec{Type: TypeH2C, Role: "internal", Target: "http://" + hostport + "/"})
	default:
		out = append(out, CheckSpec{Type: TypeHTTP, Role: "internal", Target: "http://" + hostport + "/"})
	}
	if v.HostName != "" {
		out = append(out,
			CheckSpec{Type: TypeHTTPS, Role: "public", Target: "https://" + strings.TrimSuffix(v.HostName, ".") + "/"},
			CheckSpec{Type: TypeTLS, Target: net.JoinHostPort(strings.TrimSuffix(v.HostName, "."), "443")})
	}
	return out
}

func checkKey(c CheckSpec) string { return c.Type + "|" + c.Role + "|" + c.Target }

// Sync makes the sourced endpoints match their sources: new ones appear, gone ones leave with their history, renamed
// ones keep theirs, and the admin's settings stay. A source with no checks (a UDP stream) has no endpoint yet.
func (s *Service) Sync(ctx context.Context) error {
	want, err := s.Sources(ctx)
	if err != nil {
		return err
	}
	now := s.now().UnixMilli()
	return s.st.Tx(ctx, func(q *db.Queries) error {
		rows, err := q.ListMonitorEndpoints(ctx)
		if err != nil {
			return err
		}
		have := map[string]db.MonitorEndpoint{}
		for _, r := range rows {
			if r.SourceType != SourceUser {
				have[r.SourceType+"|"+r.SourceID] = r
			}
		}
		for _, d := range want {
			key := d.SourceType + "|" + d.SourceID
			if d.Keep {
				delete(have, key)
				continue
			}
			if len(d.Checks) == 0 {
				continue
			}
			row, ok := have[key]
			delete(have, key)
			if !ok {
				row = db.MonitorEndpoint{ID: ulid.Make().String()}
				if err := q.InsertMonitorEndpoint(ctx, db.InsertMonitorEndpointParams{ID: row.ID, SourceType: d.SourceType, SourceID: d.SourceID,
					Name: d.Name, GroupName: d.Group, IntervalSeconds: 60, TimeoutSeconds: 10, StateSince: now, CreatedAt: now, UpdatedAt: now}); err != nil {
					return err
				}
			} else if row.Name != d.Name {
				if err := q.RenameMonitorEndpoint(ctx, db.RenameMonitorEndpointParams{Name: d.Name, UpdatedAt: now, ID: row.ID}); err != nil {
					return err
				}
			}
			if err := syncChecks(ctx, q, row.ID, d.Checks); err != nil {
				return err
			}
		}
		for _, gone := range have {
			if err := q.DeleteMonitorEndpoint(ctx, gone.ID); err != nil {
				return err
			}
		}
		return nil
	})
}

// syncChecks replaces the checks only when they differ, so check IDs (and the samples under them) stay stable.
func syncChecks(ctx context.Context, q *db.Queries, id string, want []CheckSpec) error {
	cur, err := q.ListMonitorChecks(ctx, id)
	if err != nil {
		return err
	}
	same := len(cur) == len(want)
	if same {
		keys := map[string]db.MonitorCheck{}
		for _, c := range cur {
			keys[c.Type+"|"+c.Role+"|"+c.Target] = c
		}
		for _, w := range want {
			c, ok := keys[checkKey(w)]
			if !ok || (c.Required == 1) != w.Required || (c.SkipVerify == 1) != w.SkipVerify {
				same = false
				break
			}
		}
	}
	if same {
		return nil
	}
	if err := q.DeleteMonitorChecks(ctx, id); err != nil {
		return err
	}
	return insertChecks(ctx, q, id, want)
}

func insertChecks(ctx context.Context, q *db.Queries, id string, want []CheckSpec) error {
	for _, w := range want {
		lo, hi := w.CodeMin, w.CodeMax
		if lo == 0 && hi == 0 {
			lo, hi = 200, 399
		}
		if err := q.InsertMonitorCheck(ctx, db.InsertMonitorCheckParams{ID: ulid.Make().String(), EndpointID: id, Type: w.Type, Role: w.Role, Target: w.Target,
			Required: flag(w.Required), CodeMin: lo, CodeMax: hi, SkipVerify: flag(w.SkipVerify), Probe: w.Probe, Expect: w.Expect}); err != nil {
			return err
		}
	}
	return nil
}

func flag(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// RunChecks is the check job: sync the endpoints, then run every due one, at most parallelMax at a time.
func (s *Service) RunChecks(ctx context.Context) (string, error) {
	// A failed sync is reported, but the endpoints that exist are still checked: monitoring outlives what it watches.
	syncErr := s.Sync(ctx)
	if syncErr != nil {
		syncErr = fmt.Errorf("sync endpoints: %w", syncErr)
	}
	rows, err := s.st.R.ListMonitorEndpoints(ctx)
	if err != nil {
		return "", err
	}
	now := s.now().UnixMilli()
	var wg sync.WaitGroup
	slots := make(chan struct{}, parallelMax)
	var mu sync.Mutex
	var firstErr error
	n := 0
	for _, e := range rows {
		if e.Paused == 1 || (e.LastCheckAt.Valid && e.LastCheckAt.Int64+e.IntervalSeconds*1000 > now+500) {
			continue
		}
		n++
		wg.Add(1)
		slots <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			if err := s.runEndpoint(ctx, e); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = fmt.Errorf("%s: %w", e.Name, err)
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return fmt.Sprintf("%d endpoint(s) checked", n), errors.Join(syncErr, firstErr)
}

// runEndpoint does one round. A round cut short by shutdown records nothing: it says nothing about the endpoint.
func (s *Service) runEndpoint(ctx context.Context, e db.MonitorEndpoint) error {
	checks, err := s.st.R.ListMonitorChecks(ctx, e.ID)
	if err != nil {
		return err
	}
	timeout := time.Duration(e.TimeoutSeconds) * time.Second
	results := make([]Result, len(checks))
	var wg sync.WaitGroup
	for i, c := range checks {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = s.exec.Run(ctx, Check{Type: c.Type, Target: c.Target, CodeMin: int(c.CodeMin), CodeMax: int(c.CodeMax), SkipVerify: c.SkipVerify == 1, Probe: c.Probe, Expect: c.Expect}, timeout)
		}()
	}
	wg.Wait()
	if ctx.Err() != nil {
		return nil
	}
	round := make([]RoundCheck, len(checks))
	var latency int64
	for i, c := range checks {
		round[i] = RoundCheck{Required: c.Required == 1, Advisory: c.Type == TypeTLS && c.Required != 1, Result: results[i]}
		if c.Type != TypeTLS {
			latency = max(latency, results[i].Latency.Milliseconds())
		}
	}
	verdict := Verdict(round, e.DegradedMs, latency)
	nowT := s.now()
	now := nowT.UnixMilli()
	return s.st.Tx(ctx, func(q *db.Queries) error {
		for i, c := range checks {
			if err := q.InsertMonitorSample(ctx, db.InsertMonitorSampleParams{EndpointID: e.ID, CheckID: c.ID, At: now, Verdict: resultVerdict(results[i]),
				Ok: flag(results[i].OK), LatencyMs: results[i].Latency.Milliseconds(), Detail: results[i].Detail}); err != nil {
				return err
			}
		}
		if err := q.InsertMonitorSample(ctx, db.InsertMonitorSampleParams{EndpointID: e.ID, CheckID: "", At: now, Verdict: verdict, Ok: flag(verdict != StateDown), LatencyMs: latency}); err != nil {
			return err
		}
		m := Machine{State: e.State, Good: e.GoodRounds, Bad: e.BadRounds}.Next(verdict)
		since := e.StateSince
		if m.State != e.State {
			since = now
			msg := transitionMessage(checks, results, e.State, m.State, nowT.Sub(time.UnixMilli(e.StateSince)))
			if err := q.InsertMonitorEvent(ctx, db.InsertMonitorEventParams{ID: ulid.Make().String(), EndpointID: e.ID, At: now, FromState: e.State, ToState: m.State, Message: msg}); err != nil {
				return err
			}
			if s.notifier != nil {
				if err := s.notifier.StateChanged(ctx, q, e, e.State, m.State, msg); err != nil {
					return err
				}
			}
		}
		return q.SetMonitorEndpointResult(ctx, db.SetMonitorEndpointResultParams{State: m.State, StateSince: since, GoodRounds: m.Good, BadRounds: m.Bad,
			LastCheckAt: sql.NullInt64{Int64: now, Valid: true}, LastLatencyMs: sql.NullInt64{Int64: latency, Valid: true}, ID: e.ID})
	})
}

func resultVerdict(r Result) string {
	switch {
	case !r.OK:
		return StateDown
	case r.Degraded:
		return StateDegraded
	}
	return StateUp
}

// transitionMessage says what changed in words, naming the failed path. It may hold internal addresses: it stays in
// the admin UI and notifications, never on public pages.
func transitionMessage(checks []db.MonitorCheck, results []Result, from, to string, lasted time.Duration) string {
	var failing []string
	for i, c := range checks {
		if !results[i].OK || results[i].Degraded {
			name := c.Type
			if c.Role != "" {
				name = c.Role + " path"
			}
			failing = append(failing, name+": "+results[i].Detail)
		}
	}
	switch {
	case to == StateUp && from == StateUnknown:
		return "First checks passed"
	case to == StateUp:
		return "Recovered after " + lasted.Round(time.Second).String()
	case len(failing) > 0:
		return strings.Join(failing, "; ")
	}
	return "Over the latency threshold"
}

// Rollup is the rollup job.
func (s *Service) Rollup(ctx context.Context) (string, error) {
	n, err := s.rollup(ctx)
	return fmt.Sprintf("%d bucket(s) written", n), err
}

// Retain is the retention job.
func (s *Service) Retain(ctx context.Context) (string, error) {
	n, err := s.retain(ctx)
	return fmt.Sprintf("%d row(s) removed", n), err
}
