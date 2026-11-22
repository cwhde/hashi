package monitor

import (
	"testing"
	"time"

	"github.com/oklog/ulid/v2"

	"git.juzo.io/juzo/hashi/internal/store/db"
)

// The 90-day strip (07 §7.1): a day with a downtime is down and names it with its times, a slow spell makes a day
// slow, a day without data says so, uptime counts slow rounds as up, and a downtime still going has no end.
func TestPublicHistoryDays(t *testing.T) {
	s, st, _, _ := fixture(t)
	ctx := t.Context()
	now := time.Date(2026, 11, 22, 13, 42, 0, 0, time.Local)
	s.now = func() time.Time { return now }
	id, err := s.CreateUser(ctx, "Home", "Home", CheckSpec{Type: TypeTCP, Target: "127.0.0.1:1"})
	if err != nil {
		t.Fatal(err)
	}
	day := func(back int, h, m int) time.Time {
		return time.Date(2026, 11, 22-back, h, m, 0, 0, time.Local)
	}
	event := func(at time.Time, from, to string) {
		t.Helper()
		if err := st.Q.InsertMonitorEvent(ctx, db.InsertMonitorEventParams{ID: ulid.Make().String(), EndpointID: id, At: at.UnixMilli(),
			FromState: from, ToState: to, Message: ""}); err != nil {
			t.Fatal(err)
		}
	}
	event(day(2, 19, 55), StateUp, StateDown)
	event(day(2, 19, 56), StateDown, StateUp)
	event(day(5, 14, 10), StateUp, StateDegraded)
	event(day(5, 14, 30), StateDegraded, StateUp)
	event(day(0, 13, 36), StateUp, StateDown)
	for back := 0; back < 10; back++ {
		for h := 0; h < 24; h++ {
			at := day(back, h, 0)
			if back == 7 || !at.Before(now.Truncate(time.Hour)) {
				continue // day 7 has no data; the current hour is not closed
			}
			ok := int64(60)
			if back == 2 && h == 19 {
				ok = 59 // one down round
			}
			if err := st.Q.UpsertMonitorRollup(ctx, db.UpsertMonitorRollupParams{EndpointID: id, CheckID: "", Resolution: 3600,
				Bucket: at.UnixMilli(), Count: 60, OkCount: ok, MinMs: 10, MaxMs: 90, AvgMs: 40, P95Ms: 80}); err != nil {
				t.Fatal(err)
			}
		}
	}
	ep, err := st.R.GetMonitorEndpoint(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	ep.State = StateDown
	h, err := s.publicHistory(ctx, st.R, map[string]*db.MonitorEndpoint{id: &ep})
	if err != nil {
		t.Fatal(err)
	}
	days := h[id].days
	if len(days) != publicDays || days[89].Date != "2026-11-22" || days[0].Date != "2026-08-25" {
		t.Fatalf("%d days from %s to %s", len(days), days[0].Date, days[len(days)-1].Date)
	}
	down := days[87]
	if down.State != StateDown || len(down.Spells) != 1 || !down.Spells[0].Start.Equal(day(2, 19, 55)) || !down.Spells[0].End.Equal(day(2, 19, 56)) {
		t.Fatalf("20 Nov: %+v", down)
	}
	if down.Uptime == nil || *down.Uptime > 99.99 || *down.Uptime < 99.9 {
		t.Fatalf("20 Nov uptime %v, want one round of 1440 down", down.Uptime)
	}
	if slow := days[84]; slow.State != StateDegraded || slow.Uptime == nil || *slow.Uptime != 100 {
		t.Fatalf("17 Nov: %+v, slow counts as up", slow)
	}
	if empty := days[82]; empty.State != "unknown" || empty.Uptime != nil {
		t.Fatalf("15 Nov: %+v, want no data", empty)
	}
	if today := days[89]; today.State != StateDown || len(today.Spells) != 1 || !today.Spells[0].End.IsZero() {
		t.Fatalf("today: %+v, want a downtime still going", today)
	}
	if quiet := days[88]; quiet.State != StateUp || len(quiet.Spells) != 0 {
		t.Fatalf("21 Nov: %+v", quiet)
	}
}
