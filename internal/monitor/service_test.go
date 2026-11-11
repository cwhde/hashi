package monitor

import (
	"context"
	"database/sql"
	"errors"
	"net/netip"
	"sync"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/ipsets"
	"git.juzo.io/juzo/hashi/internal/services"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/store"
	"git.juzo.io/juzo/hashi/internal/store/db"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
)

type recorder struct{ changes []string }

func (r *recorder) StateChanged(_ context.Context, _ *db.Queries, e db.MonitorEndpoint, from, to, _ string) error {
	r.changes = append(r.changes, e.Name+":"+from+">"+to)
	return nil
}

func fixture(t *testing.T) (*Service, *store.Store, *recorder, *error) {
	t.Helper()
	st := storetest.New(t)
	var down error
	rec := &recorder{}
	s := New(st, func(context.Context) error { return down }, nil, rec)
	var mu sync.Mutex // checks run concurrently and each reads the clock
	clock := time.Now()
	s.now = func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		clock = clock.Add(time.Minute)
		return clock
	}
	return s, st, rec, &down
}

// A real outage of Hashi's own endpoint: three failed rounds go down once, two good ones recover once, and each change
// is an event and a notification.
func TestRoundsDriveStateAndNotify(t *testing.T) {
	s, st, rec, down := fixture(t)
	ctx := t.Context()
	round := func() {
		t.Helper()
		if _, err := s.RunChecks(ctx); err != nil {
			t.Fatal(err)
		}
	}
	round()
	round()
	*down = errors.New("database gone")
	round()
	round()
	round()
	round()
	*down = nil
	round()
	round()
	want := []string{"Hashi:unknown>up", "Hashi:up>down", "Hashi:down>up"}
	if len(rec.changes) != len(want) || rec.changes[1] != want[1] || rec.changes[2] != want[2] {
		t.Fatalf("changes %v", rec.changes)
	}
	rows, err := s.List(ctx)
	if err != nil || len(rows) != 1 || rows[0].State != StateUp {
		t.Fatalf("list %+v %v", rows, err)
	}
	ev, err := st.R.ListMonitorEvents(ctx, db.ListMonitorEventsParams{EndpointID: rows[0].Endpoint.ID, Limit: 10})
	if err != nil || len(ev) != 3 {
		t.Fatalf("events %+v %v", ev, err)
	}
}

// The public view holds no internal detail even when the endpoint failed with one (07 §7): only marked endpoints appear.
func TestPublicStatusOnlyPublicEndpoints(t *testing.T) {
	s, _, _, down := fixture(t)
	ctx := t.Context()
	*down = errors.New("connect 10.0.0.9:443 refused")
	if _, err := s.RunChecks(ctx); err != nil {
		t.Fatal(err)
	}
	v, err := s.PublicStatus(ctx)
	if err != nil || len(v.Endpoints) != 0 {
		t.Fatalf("hidden endpoint shown: %+v %v", v, err)
	}
	rows, _ := s.List(ctx)
	e := rows[0].Endpoint
	if err := s.Update(ctx, e.ID, Settings{Group: "G", IntervalSeconds: 60, TimeoutSeconds: 5, Public: true}); err != nil {
		t.Fatal(err)
	}
	s.statusCache = cache[PublicStatus]{}
	if v, err = s.PublicStatus(ctx); err != nil || len(v.Endpoints) != 1 || v.Endpoints[0].Group != "G" {
		t.Fatalf("public %+v %v", v, err)
	}
}

func TestUserMonitorRules(t *testing.T) {
	s, _, _, _ := fixture(t)
	ctx := t.Context()
	if _, err := s.CreateUser(ctx, "x", "", CheckSpec{Type: TypeUDP, Target: "127.0.0.1:1"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("udp without a probe: %v", err)
	}
	if _, err := s.CreateUser(ctx, "x", "", CheckSpec{Type: TypeHTTP, Target: "https://a"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("http with https address: %v", err)
	}
	id, err := s.CreateUser(ctx, "x", "", CheckSpec{Type: TypeTCP, Target: "127.0.0.1:1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, id, "1h"); err != nil {
		t.Fatalf("sync removed a user monitor: %v", err)
	}
	rows, _ := s.List(ctx)
	for _, r := range rows {
		if r.Endpoint.SourceType == SourceHashi {
			if err := s.DeleteUser(ctx, r.Endpoint.ID); !errors.Is(err, ErrSourced) {
				t.Fatalf("delete sourced: %v", err)
			}
		}
	}
	if err := s.DeleteUser(ctx, id); err != nil {
		t.Fatal(err)
	}
}

// Review of S7: one service's address set failing stops nothing: its endpoint stays, the others are checked.
func TestResolverErrorStopsNothing(t *testing.T) {
	s, st, _, _ := fixture(t)
	ctx := t.Context()
	set, err := ipsets.CreateSet(ctx, st.Q, ipsets.SetInput{Slug: "wiki", Name: "Wiki"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := services.Create(ctx, st.Q, services.Service{Name: "Wiki", Slug: "wiki", Kind: services.KindTCP, DomainMode: services.ModeNone, Enabled: true, MonitoringVisible: true, Origin: services.OriginUser,
		PublicPort: 2222, Target: services.Target{SetID: set.ID, Port: 1}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	s.resolve = func(context.Context, string, string) (ipsets.Target, error) {
		return ipsets.Target{Addresses: []netip.Addr{netip.MustParseAddr("127.0.0.1")}}, nil
	}
	if _, err := s.RunChecks(ctx); err != nil {
		t.Fatal(err)
	}
	s.resolve = func(context.Context, string, string) (ipsets.Target, error) {
		return ipsets.Target{}, errors.New("set broken")
	}
	if _, err := s.RunChecks(ctx); err != nil {
		t.Fatal(err)
	}
	rows, _ := s.List(ctx)
	if len(rows) != 2 {
		t.Fatalf("%d endpoints", len(rows))
	}
}

// Review of S7, 01 §4.3: retention removes fewer than 5000 rows at a time and still removes them all.
func TestRetentionBatches(t *testing.T) {
	s, st, _, _ := fixture(t)
	ctx := t.Context()
	now := s.now()
	if err := st.Q.InsertMonitorEndpoint(ctx, db.InsertMonitorEndpointParams{ID: "e", SourceType: SourceUser, Name: "e", GroupName: "g", IntervalSeconds: 60, TimeoutSeconds: 5}); err != nil {
		t.Fatal(err)
	}
	old := now.Add(-40 * 24 * time.Hour).UnixMilli()
	err := st.Tx(ctx, func(q *db.Queries) error {
		for i := range int64(12000) {
			if err := q.InsertMonitorSample(ctx, db.InsertMonitorSampleParams{EndpointID: "e", At: old + i, Verdict: StateUp, Ok: 1}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if end, err := st.R.SampleBatchEnd(ctx, now.UnixMilli()); err != nil || end != old+4999 {
		t.Fatalf("batch end %d %v", end, err)
	}
	if n, err := s.retain(ctx); err != nil || n != 12000 {
		t.Fatalf("removed %d %v", n, err)
	}
}

// AdGuard and Pulse are watched too (07 §1): a revoked or never-seen agent is no outage, a silent one is.
func TestAdGuardAndPulseSources(t *testing.T) {
	s, st, _, _ := fixture(t)
	ctx := t.Context()
	now := time.Now().UnixMilli()
	if err := st.Q.InsertPulseAgent(ctx, db.InsertPulseAgentParams{ID: "new", Name: "new", TokenHash: []byte("x"), InstallType: "linux", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := st.Q.InsertPulseAgent(ctx, db.InsertPulseAgentParams{ID: "seen", Name: "seen", TokenHash: []byte("y"), InstallType: "linux", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	beat := func(at time.Time) {
		t.Helper()
		if err := st.Q.SetPulseHeartbeat(ctx, db.SetPulseHeartbeatParams{LastSeenAt: sql.NullInt64{Int64: at.UnixMilli(), Valid: true}, CandidatesJson: "[]", IntervalSeconds: 60, InstallType: "linux", UpdatedAt: now, ID: "seen"}); err != nil {
			t.Fatal(err)
		}
	}
	beat(time.Now())
	got, err := s.Sources(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var agents []string
	for _, d := range got {
		if d.SourceType == SourcePulse {
			agents = append(agents, d.SourceID)
		}
	}
	if len(agents) != 1 || agents[0] != "seen" {
		t.Fatalf("%v", agents)
	}
	check := Check{Type: TypeSelf, Target: PulseTargetPrefix + "seen"}
	if r := s.exec.Run(ctx, check, time.Second); !r.OK {
		t.Fatalf("fresh agent: %+v", r)
	}
	beat(time.Now().Add(-time.Hour))
	if r := s.exec.Run(ctx, check, time.Second); r.OK {
		t.Fatal("a silent agent must fail its check")
	}
}

// 08 §4 step 6: the mark stays through a failed round, while the state is still up, and goes with the next passing one.
func TestPendingDNSClearsOnlyOnAPassingCheck(t *testing.T) {
	s, st, _, down := fixture(t)
	ctx := t.Context()
	round := func() {
		t.Helper()
		if _, err := s.RunChecks(ctx); err != nil {
			t.Fatal(err)
		}
	}
	round()
	pending := func() int64 {
		t.Helper()
		rows, err := st.R.ListMonitorEndpoints(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range rows {
			if r.SourceType == SourceHashi {
				return r.PendingDns
			}
		}
		t.Fatal("no endpoint")
		return 0
	}
	if err := st.Q.MarkMonitorPendingDNS(ctx, db.MarkMonitorPendingDNSParams{SourceType: SourceHashi, SourceID: hashiEndpointSource(t, st)}); err != nil {
		t.Fatal(err)
	}
	*down = errors.New("not yet")
	round()
	if pending() != 1 {
		t.Fatal("a failed round must not clear it")
	}
	*down = nil
	round()
	if pending() != 0 {
		t.Fatal("a passing round clears it")
	}
}

// 07 §1: AdGuard is watched at each address its connection would try; a disabled one is not watched.
func TestAdGuardSource(t *testing.T) {
	s, st, _, _ := fixture(t)
	ctx := t.Context()
	s.resolve = func(context.Context, string, string) (ipsets.Target, error) {
		return ipsets.Target{Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.5"), netip.MustParseAddr("10.0.0.6")}}, nil
	}
	if err := st.Q.InsertIPSet(ctx, db.InsertIPSetParams{ID: "t", Slug: "t", Name: "t", Kind: "user", Origin: "user"}); err != nil {
		t.Fatal(err)
	}
	for id, mode := range map[string]string{"on": "read_only", "off": "disabled"} {
		if err := st.Q.InsertAdGuardConnection(ctx, db.InsertAdGuardConnectionParams{ID: id, Name: id, TargetSetID: "t", Port: 3000, Scheme: "http", Mode: mode}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.Sources(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var seen []Desired
	for _, d := range got {
		if d.SourceType == SourceAdGuard {
			seen = append(seen, d)
		}
	}
	if len(seen) != 1 || seen[0].SourceID != "on" || len(seen[0].Checks) != 2 || seen[0].Checks[0].Required || seen[0].Checks[1].Target != "10.0.0.6:3000" {
		t.Fatalf("%+v", seen)
	}
}

func hashiEndpointSource(t *testing.T, st *store.Store) string {
	t.Helper()
	rows, err := st.R.ListMonitorEndpoints(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.SourceType == SourceHashi {
			return r.SourceID
		}
	}
	t.Fatal("no endpoint")
	return ""
}

// The Monitoring settings (09 §2) reach new endpoints, the healthy-code range of new checks and the sample retention;
// endpoints that exist keep their own values.
func TestMonitoringSettingsApply(t *testing.T) {
	s, st, _, _ := fixture(t)
	ctx := t.Context()
	s.UseDefaults(settings.Monitoring{IntervalSeconds: 120, TimeoutSeconds: 7, CodeMin: 200, CodeMax: 299, DegradedMs: 800, RetentionDays: 2})
	id, err := s.CreateUser(ctx, "api", "", CheckSpec{Type: TypeHTTPS, Target: "https://example.com/health"})
	if err != nil {
		t.Fatal(err)
	}
	e, err := st.R.GetMonitorEndpoint(ctx, id)
	if err != nil || e.IntervalSeconds != 120 || e.TimeoutSeconds != 7 || e.DegradedMs != 800 {
		t.Fatalf("endpoint %+v %v", e, err)
	}
	checks, err := st.R.ListMonitorChecks(ctx, id)
	if err != nil || len(checks) != 1 || checks[0].CodeMin != 200 || checks[0].CodeMax != 299 {
		t.Fatalf("checks %+v %v", checks, err)
	}
	// Samples older than 2 days go; a 1-day-old one stays.
	now := s.now()
	for i, age := range []time.Duration{3 * 24 * time.Hour, 24 * time.Hour} {
		if err := st.Q.InsertMonitorSample(ctx, db.InsertMonitorSampleParams{EndpointID: id, At: now.Add(-age).UnixMilli() + int64(i), Verdict: StateUp, Ok: 1}); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := s.retain(ctx); err != nil || n != 1 {
		t.Fatalf("removed %d %v", n, err)
	}
}
