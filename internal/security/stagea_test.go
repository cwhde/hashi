package security

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/netip"
	"sync"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/store/db"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
)

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func testEngine(t *testing.T) (*Engine, *clock) {
	t.Helper()
	st := storetest.New(t)
	c := &clock{t: time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)}
	e := NewEngine(st, slog.New(slog.NewTextHandler(io.Discard, nil)), c.now)
	cfg := e.Settings()
	cfg.RateRequests, cfg.RateWindowSeconds, cfg.ChallengeMaxFailed = 10, 10, 3
	e.UseSettings(cfg)
	return e, c
}

func run(t *testing.T, e *Engine, r Request) Outcome {
	t.Helper()
	o, err := e.StageA(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	return o
}

var bad = netip.MustParseAddr("198.51.100.7")

func TestRateTriggerChallengeAndEscalation(t *testing.T) {
	ctx := context.Background()
	e, _ := testEngine(t)
	for i := range 10 {
		if o := run(t, e, Request{Addr: bad}); o.Step != Pass {
			t.Fatalf("request %d: %+v", i, o)
		}
	}
	if o := run(t, e, Request{Addr: bad}); o.Step != Challenged || o.Stage != "A2" {
		t.Fatalf("11th request: %+v", o)
	}
	// Requests to the challenge flow never count.
	for range 5 {
		if o := run(t, e, Request{Addr: bad, ChallengeFlow: true}); o.Step != Pass {
			t.Fatalf("challenge flow: %+v", o)
		}
	}
	for f := 1; f < 3; f++ {
		if o := run(t, e, Request{Addr: bad}); o.Step != Challenged || o.Stage != "A3" || o.Failures != f {
			t.Fatalf("failure %d: %+v", f, o)
		}
	}
	if o := run(t, e, Request{Addr: bad}); o.Step != BannedNow {
		t.Fatalf("F_max: %+v", o)
	}
	if !e.Banned(bad) || len(e.Challenges()) != 0 {
		t.Fatal("not moved from challenge to blocked")
	}
	// A1 now answers before A2 counts anything.
	if o := run(t, e, Request{Addr: bad}); o.Step != Blocked || o.Stage != "A1" {
		t.Fatalf("after ban: %+v", o)
	}
	c, err := e.st.R.GetAddressCounters(ctx, "198.51.100.7")
	if err != nil || c.BanCount != 1 || c.ChallengeCount != 1 || c.LastSource != SourceChallengeFailed {
		t.Fatalf("counters %+v %v", c, err)
	}
	b := e.Bans()[0]
	if got := b.Expires.Sub(b.Created); got != 15*time.Minute {
		t.Fatalf("first ban lasts %v", got)
	}
}

func TestSSOExemptFromRateOnly(t *testing.T) {
	ctx := context.Background()
	e, _ := testEngine(t)
	for range 50 {
		if o := run(t, e, Request{Addr: bad, SSO: true}); o.Step != Pass {
			t.Fatalf("SSO request: %+v", o)
		}
	}
	// WAF matches still count with a session.
	for range 5 {
		if err := e.WAFMatch(ctx, bad, "", WAFDetail{}); err != nil {
			t.Fatal(err)
		}
	}
	if o := run(t, e, Request{Addr: bad, SSO: true}); o.Step != Challenged || o.Stage != "A3" {
		t.Fatalf("after WAF matches: %+v", o)
	}
}

func TestSolveKeepsBanCounter(t *testing.T) {
	ctx := context.Background()
	e, c := testEngine(t)
	if _, err := e.Ban(ctx, bad, Cause{Source: SourceManual}); err != nil {
		t.Fatal(err)
	}
	c.add(16 * time.Minute) // first ban over
	if e.Banned(bad) {
		t.Fatal("ban did not end")
	}
	if ok, _ := e.Challenge(ctx, bad, Cause{Source: SourceManual}); !ok {
		t.Fatal("not challenged")
	}
	if ok, _ := e.Solve(ctx, bad); !ok {
		t.Fatal("not solved")
	}
	if o := run(t, e, Request{Addr: bad}); o.Step != Pass {
		t.Fatalf("after solve: %+v", o)
	}
	if _, err := e.Ban(ctx, bad, Cause{Source: SourceManual}); err != nil {
		t.Fatal(err)
	}
	b := e.Bans()[0]
	if got := b.Expires.Sub(b.Created); got != 30*time.Minute {
		t.Fatalf("second ban lasts %v (counter reset by the solve?)", got)
	}
}

func TestBanCounterDecaysAfterBanEnds(t *testing.T) {
	ctx := context.Background()
	e, c := testEngine(t)
	if _, err := e.Ban(ctx, bad, Cause{Source: SourceManual}); err != nil {
		t.Fatal(err)
	}
	// Quiet time starts at the end of the ban: 15 min + one decay period later, the next ban is a first ban again.
	c.add(15*time.Minute + 7*24*time.Hour)
	if _, err := e.Ban(ctx, bad, Cause{Source: SourceManual}); err != nil {
		t.Fatal(err)
	}
	b := e.Bans()[0]
	if got := b.Expires.Sub(b.Created); got != 15*time.Minute {
		t.Fatalf("ban after decay lasts %v", got)
	}
}

func TestPermanentBanNeverDecays(t *testing.T) {
	ctx := context.Background()
	e, c := testEngine(t)
	cfg := e.Settings()
	cfg.BanPermanentAfter = 1
	e.UseSettings(cfg)
	if _, err := e.Ban(ctx, bad, Cause{Source: SourceManual}); err != nil {
		t.Fatal(err)
	}
	c.add(400 * 24 * time.Hour)
	if !e.Banned(bad) || !e.Bans()[0].Expires.IsZero() {
		t.Fatal("permanent ban ended")
	}
	cnt, _ := e.st.R.GetAddressCounters(ctx, "198.51.100.7")
	if Effective(int(cnt.BanCount), time.UnixMilli(cnt.BanQuietSince), c.now(), 7*24*time.Hour) != 1 {
		t.Fatal("permanent ban's counter decayed")
	}
}

func TestInvariant_20_ManualAllowBeatsDynamic_L7(t *testing.T) {
	ctx := context.Background()
	e, _ := testEngine(t)
	e.UseAllowed(func(a netip.Addr) bool { return a == bad })
	for range 100 {
		if o := run(t, e, Request{Addr: bad, StaticBlock: true}); o.Step != Pass {
			t.Fatalf("allowed address: %+v", o)
		}
	}
	for range 20 {
		_ = e.WAFMatch(ctx, bad, "", WAFDetail{})
	}
	if ok, _ := e.Challenge(ctx, bad, Cause{Source: SourceManual}); ok {
		t.Fatal("allowed address challenged")
	}
	if ok, _ := e.Ban(ctx, bad, Cause{Source: SourceMeter}); ok {
		t.Fatal("allowed address banned")
	}
	if len(e.Bans())+len(e.Challenges()) != 0 {
		t.Fatal("allowed address in a dynamic set")
	}
	e.ev.drain(ctx)
	evs, _ := e.st.R.ListSecurityEventsForAddress(ctx, db.ListSecurityEventsForAddressParams{Address: KeyString(bad), Limit: 10})
	skipped := 0
	for _, x := range evs {
		if x.EventType == "ban_skipped_allowed" {
			skipped++
		}
	}
	if skipped != 1 {
		t.Fatalf("skip not recorded: %+v", evs)
	}
}

func TestInvariant_StageOrder_A1BeforeA2BeforeA3(t *testing.T) {
	ctx := context.Background()
	e, _ := testEngine(t)
	// Statically blocked: A1 answers, A2 never counts it.
	for range 30 {
		if o := run(t, e, Request{Addr: bad, StaticBlock: true}); o.Step != Blocked || o.Stage != "A1" {
			t.Fatalf("static block: %+v", o)
		}
	}
	if len(e.Challenges()) != 0 {
		t.Fatal("A2 counted a blocked address")
	}
	// Challenged and over the rate: A3 answers (failures), A2 does not re-trigger.
	if _, err := e.Challenge(ctx, bad, Cause{Source: SourceManual}); err != nil {
		t.Fatal(err)
	}
	if o := run(t, e, Request{Addr: bad}); o.Stage != "A3" || o.Failures != 1 {
		t.Fatalf("challenged: %+v", o)
	}
}

func TestIPv6PerSlash64(t *testing.T) {
	ctx := context.Background()
	e, _ := testEngine(t)
	if _, err := e.Ban(ctx, netip.MustParseAddr("2001:db8:1:2::1"), Cause{Source: SourceManual}); err != nil {
		t.Fatal(err)
	}
	if !e.Banned(netip.MustParseAddr("2001:db8:1:2:ffff::9")) || e.Banned(netip.MustParseAddr("2001:db8:1:3::1")) {
		t.Fatal("ban is not per /64")
	}
	if got := e.Bans()[0].Prefix.String(); got != "2001:db8:1:2::/64" {
		t.Fatal(got)
	}
}

func TestChallengeExpiresAndLoadRestores(t *testing.T) {
	ctx := context.Background()
	e, c := testEngine(t)
	if _, err := e.Challenge(ctx, bad, Cause{Source: SourceManual}); err != nil {
		t.Fatal(err)
	}
	other := netip.MustParseAddr("198.51.100.8")
	if _, err := e.Ban(ctx, other, Cause{Source: SourceManual}); err != nil {
		t.Fatal(err)
	}
	run(t, e, Request{Addr: bad}) // f = 1
	if _, err := e.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	// A restart reads both sets back, failures included.
	e2 := NewEngine(e.st, e.log, c.now)
	e2.UseSettings(e.Settings())
	if err := e2.Load(ctx); err != nil {
		t.Fatal(err)
	}
	if ch := e2.Challenges(); len(ch) != 1 || ch[0].Failures != 1 || !e2.Banned(other) {
		t.Fatalf("after load: %+v", ch)
	}
	c.add(31 * time.Minute) // T_exp = 30 min
	if o := run(t, e2, Request{Addr: bad}); o.Step != Pass {
		t.Fatalf("expired challenge still enforced: %+v", o)
	}
}

// Closed minutes are written without addresses; path prefixes are cut at the first segment and capped per service.
func TestBucketsFlushClosedMinutes(t *testing.T) {
	ctx := context.Background()
	e, c := testEngine(t)
	at := c.now()
	e.Record(Hit{At: at, Addr: bad, ServiceID: "", Decision: DecisionAllow, Path: "/api/v1/x"})
	e.Record(Hit{At: at, Addr: bad, ServiceID: "", Decision: DecisionAllow, Path: "/api/v2"})
	e.Record(Hit{At: at, Addr: bad, ServiceID: "", Decision: DecisionChallenge, Path: "/"})
	if _, err := e.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if rows, _ := e.st.R.ListSecurityBuckets(ctx, 0); len(rows) != 0 {
		t.Fatalf("open minute written: %+v", rows)
	}
	c.add(time.Minute)
	if _, err := e.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	rows, err := e.st.R.ListSecurityBuckets(ctx, 0)
	if err != nil || len(rows) != 2 {
		t.Fatalf("buckets %+v %v", rows, err)
	}
	for _, r := range rows {
		switch r.Decision {
		case DecisionAllow:
			if r.PathPrefix != "/api" || r.RequestCount != 2 {
				t.Fatalf("allow bucket %+v", r)
			}
		case DecisionChallenge:
			if r.ChallengedCount != 1 {
				t.Fatalf("challenge bucket %+v", r)
			}
		}
	}
}

// Access-log lines add response rows with the status class (06 §6.1): their own decision, so the forward-auth decision
// counts stay as they are, and the top addresses aren't counted twice.
func TestBucketsCountResponsesByStatusClass(t *testing.T) {
	ctx := context.Background()
	e, c := testEngine(t)
	at := c.now()
	e.Record(Hit{At: at, Addr: bad, ServiceID: "s1", Decision: DecisionAllow, Path: "/"})
	for _, st := range []int{200, 204, 404, 502} {
		e.RecordResponse(Response{At: at, Addr: bad, ServiceID: "s1", Path: "/", Status: st})
	}
	c.add(time.Minute)
	if _, err := e.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	rows, err := e.st.R.ListSecurityBuckets(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int64{}
	for _, r := range rows {
		got[r.Decision+"/"+r.StatusClass] += r.RequestCount
	}
	want := map[string]int64{"allow/": 1, "response/2xx": 2, "response/4xx": 1, "response/5xx": 1}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("buckets %v, want %v", got, want)
	}
	var n int64
	if err := e.st.Reader.QueryRowContext(ctx, "SELECT sum(request_count) FROM security_ip_top_1m").Scan(&n); err != nil || n != 1 {
		t.Fatalf("top addresses count forward-auth decisions only: %d %v", n, err)
	}
}

// An address banned before it became allowed (the admin's own address, a meter trip from home) is lifted: once at
// start and by the sweep, so a ban never outlives the allow (invariant 20).
func TestInvariant_20_AllowLiftsExistingBan(t *testing.T) {
	ctx := context.Background()
	e, _ := testEngine(t)
	allowed := false
	e.UseAllowed(func(a netip.Addr) bool { return allowed && a == bad })
	if ok, err := e.Ban(ctx, bad, Cause{Source: SourceMeter, Reason: "tripped an L4 meter"}); !ok || err != nil {
		t.Fatalf("ban %v %v", ok, err)
	}
	if n, _ := e.LiftAllowed(ctx); n != 0 || !e.Banned(bad) {
		t.Fatal("lifted a ban of an address that is not allowed")
	}
	allowed = true
	if _, err := e.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if e.Banned(bad) || len(e.Bans()) != 0 {
		t.Fatal("ban of an allowed address survived the sweep")
	}
	// Stored too: a restart does not bring it back.
	e2 := NewEngine(e.st, slog.New(slog.NewTextHandler(io.Discard, nil)), time.Now)
	if err := e2.Load(ctx); err != nil || e2.Banned(bad) {
		t.Fatalf("after reload banned=%v err=%v", e2.Banned(bad), err)
	}
}
