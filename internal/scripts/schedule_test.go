package scripts

import (
	"bufio"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

// captured reads testdata/contracts/scripts/calendar_debian13.txt: expression → firing times systemd computed.
func captured(t *testing.T) map[string][]time.Time {
	t.Helper()
	f, err := os.Open("../../testdata/contracts/scripts/calendar_debian13.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	out, cur := map[string][]time.Time{}, ""
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if e, ok := strings.CutPrefix(l, "-- "); ok {
			cur = e
			continue
		}
		_, v, ok := strings.Cut(l, ": ")
		if !ok || (!strings.HasPrefix(l, "Next elapse") && !strings.HasPrefix(l, "Iteration")) {
			continue
		}
		at, err := time.Parse("Mon 2006-01-02 15:04:05 MST", v)
		if err != nil {
			t.Fatalf("%q: %v", l, err)
		}
		out[cur] = append(out[cur], at.UTC())
	}
	return out
}

// The conversion fires exactly when cron would: for every expression, the union of systemd's captured firing times for
// the emitted OnCalendar lines equals cron's own next times (09 §1 round-trip test).
func TestOnCalendarRoundTrip(t *testing.T) {
	sys := captured(t)
	base := time.Date(2026, 11, 11, 10, 0, 0, 0, time.UTC)
	for _, tc := range []struct{ cron string }{
		{"30 3 * * 0"}, {"*/15 * * * *"}, {"0 4 1 * *"}, {"0 0 * * 1-5"}, {"0 12 1 * 1"}, {"@daily"},
		{"5,35 */6 * * *"}, {"0 9-17/4 * 1,6 *"}, {"0 0 13 * 5"}, {"15 2 * * 7"},
	} {
		s, err := ParseCron(tc.cron)
		if err != nil {
			t.Fatalf("%s: %v", tc.cron, err)
		}
		var union []time.Time
		for _, c := range s.OnCalendar() {
			got, ok := sys[c]
			if !ok {
				t.Fatalf("%s: %q was not captured; add it to calendar.sh and capture again", tc.cron, c)
			}
			union = append(union, got...)
		}
		slices.SortFunc(union, time.Time.Compare)
		union = slices.Compact(union)[:5] // each line has 5 captured times, so the first 5 of the union are complete
		if want := s.Next(base, 5); !slices.Equal(union, want) {
			t.Errorf("%s → %q\n systemd %v\n cron    %v", tc.cron, s.OnCalendar(), union, want)
		}
	}
}

func TestParseCronRefuses(t *testing.T) {
	for _, e := range []string{"", "* * * *", "60 * * * *", "* 24 * * *", "* * 0 * *", "* * * 13 *", "* * * * 8",
		"*/0 * * * *", "5-1 * * * *", "a * * * *", "@reboot"} {
		if _, err := ParseCron(e); err == nil {
			t.Errorf("%q accepted", e)
		}
	}
}
