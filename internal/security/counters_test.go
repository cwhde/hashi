package security

import (
	"context"
	"net/netip"
	"testing"
	"time"
)

// The counters view (06 §13.1): effective values after decay, the live state, filters and sorting; edits count from
// now, a reset zeroes both, and resetting the failures restarts a live challenge's count.
func TestCountersViewEditAndReset(t *testing.T) {
	ctx := context.Background()
	e, c := testEngine(t)
	other := netip.MustParseAddr("203.0.113.9")
	v6 := netip.MustParseAddr("2001:db8:1:2::5")
	if ok, err := e.Ban(ctx, other, Cause{Source: SourceMeter, Reason: "meter"}); !ok || err != nil {
		t.Fatalf("ban %v %v", ok, err)
	}
	if ok, err := e.Challenge(ctx, bad, Cause{Source: SourceRate}); !ok || err != nil {
		t.Fatalf("challenge %v %v", ok, err)
	}
	if ok, err := e.Challenge(ctx, v6, Cause{Source: SourceWAF}); !ok || err != nil {
		t.Fatalf("challenge v6 %v %v", ok, err)
	}
	run(t, e, Request{Addr: bad}) // one unsolved request
	c.add(time.Second)

	rows, err := e.Counters(ctx, CounterFilter{State: "active", Sort: "ban"})
	if err != nil || len(rows) != 3 {
		t.Fatalf("active %+v %v", rows, err)
	}
	if rows[0].Address != other.String() || rows[0].State != StateBanned || rows[0].Ban != 1 || rows[0].Until.IsZero() {
		t.Fatalf("banned first by ban counter: %+v", rows[0])
	}
	got := map[string]CounterRow{}
	for _, r := range rows {
		got[r.Address] = r
	}
	if r := got[bad.String()]; r.State != StateChallenged || r.Challenge != 1 || r.Failures != 1 || r.MaxFailures != 3 {
		t.Fatalf("challenged %+v", r)
	}
	if _, ok := got["2001:db8:1:2::/64"]; !ok {
		t.Fatalf("IPv6 counts per /64: %+v", got)
	}
	for _, tc := range []struct {
		f    CounterFilter
		want int
	}{
		{CounterFilter{State: StateBanned}, 1},
		{CounterFilter{Source: SourceWAF}, 1},
		{CounterFilter{MinBan: 1}, 1},
		{CounterFilter{Within: netip.MustParsePrefix("198.51.100.0/24")}, 1},
		{CounterFilter{Within: netip.MustParsePrefix("2001:db8::/32")}, 1},
		{CounterFilter{Since: c.now().Add(time.Hour)}, 0},
	} {
		if rows, _ := e.Counters(ctx, tc.f); len(rows) != tc.want {
			t.Errorf("%+v: %d rows, want %d", tc.f, len(rows), tc.want)
		}
	}

	five := 5
	before, err := e.EditCounters(ctx, bad, CounterEdit{Ban: &five, ResetFailures: true}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	// The values it replaced, as the counters view showed them (06 §16 before/after).
	if before != (CounterValues{Address: bad.String(), Ban: 0, Challenge: 1}) {
		t.Fatalf("before %+v", before)
	}
	rows, _ = e.Counters(ctx, CounterFilter{Within: netip.PrefixFrom(bad, 32)})
	if r := rows[0]; r.Ban != 5 || r.Challenge != 1 || r.Failures != 0 {
		t.Fatalf("edited %+v", r)
	}
	// The set value decays from the edit on: one step per quiet period.
	c.add(time.Duration(e.Settings().DecaySeconds) * time.Second)
	rows, _ = e.Counters(ctx, CounterFilter{Within: netip.PrefixFrom(bad, 32)})
	if rows[0].Ban != 4 {
		t.Fatalf("decay after the edit: %+v", rows[0])
	}
	prev, err := e.ResetCounters(ctx, []string{bad.String(), other.String(), "192.0.2.99"}, "admin")
	if err != nil || len(prev) != 2 {
		t.Fatalf("reset %+v %v", prev, err)
	}
	// The decayed values just before the reset, one per existing record; the unknown address has none.
	if prev[0] != (CounterValues{Address: bad.String(), Ban: 4}) || prev[1].Address != other.String() {
		t.Fatalf("reset before %+v", prev)
	}
	if rows, _ := e.Counters(ctx, CounterFilter{MinBan: 1}); len(rows) != 0 {
		t.Fatalf("after reset %+v", rows)
	}
}

// A ban counter set by hand while a ban runs starts its quiet time when that ban ends, not now (06 §5.1, §18 "running
// ban unchanged"): the counter must not decay during the ban.
func TestEditDuringARunningBanDecaysAfterIt(t *testing.T) {
	ctx := context.Background()
	e, c := testEngine(t)
	d := time.Duration(e.Settings().DecaySeconds) * time.Second
	if ok, err := e.Ban(ctx, bad, Cause{Source: SourceManual, Duration: 5 * d}); !ok || err != nil {
		t.Fatalf("ban %v %v", ok, err)
	}
	three := 3
	if _, err := e.EditCounters(ctx, bad, CounterEdit{Ban: &three}, "admin"); err != nil {
		t.Fatal(err)
	}
	ban := func() int {
		t.Helper()
		rows, err := e.Counters(ctx, CounterFilter{Within: netip.PrefixFrom(bad, 32)})
		if err != nil || len(rows) != 1 {
			t.Fatalf("rows %+v %v", rows, err)
		}
		return rows[0].Ban
	}
	c.add(2 * d) // still inside the ban
	if got := ban(); got != 3 {
		t.Fatalf("decayed during the running ban: %d, want 3", got)
	}
	c.add(3*d + d) // the ban ended 3d after the edit point; one quiet period after it
	if got := ban(); got != 2 {
		t.Fatalf("one quiet period after the ban: %d, want 2", got)
	}
}
