package security

import (
	"net/netip"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/settings"
)

func TestBanDuration(t *testing.T) {
	m := time.Minute
	cases := []struct {
		name string
		p    BanParams
		c    int
		want time.Duration
		perm bool
	}{
		{"constant", BanParams{Function: settings.BanConstant, Base: 15 * m, Max: 30 * 24 * time.Hour, K: 2}, 7, 15 * m, false},
		{"linear first", BanParams{Function: settings.BanLinear, Base: 15 * m, Max: time.Hour * 1000, K: 2}, 1, 15 * m, false},
		{"linear third", BanParams{Function: settings.BanLinear, Base: 15 * m, Max: time.Hour * 1000, K: 2}, 3, 75 * m, false},
		{"exponential first", BanParams{Function: settings.BanExponential, Base: 15 * m, Max: 30 * 24 * time.Hour, K: 2}, 1, 15 * m, false},
		{"exponential fourth", BanParams{Function: settings.BanExponential, Base: 15 * m, Max: 30 * 24 * time.Hour, K: 2}, 4, 120 * m, false},
		{"capped", BanParams{Function: settings.BanExponential, Base: 15 * m, Max: 6 * time.Hour, K: 2}, 6, 6 * time.Hour, false},
		{"no overflow", BanParams{Function: settings.BanExponential, Base: 15 * m, Max: 30 * 24 * time.Hour, K: 100}, 400, 30 * 24 * time.Hour, false},
		{"permanent", BanParams{Function: settings.BanExponential, Base: 15 * m, Max: time.Hour, K: 2, PermanentAfter: 5}, 5, 0, true},
		{"before permanent", BanParams{Function: settings.BanExponential, Base: 15 * m, Max: time.Hour, K: 2, PermanentAfter: 5}, 4, time.Hour, false},
	}
	for _, c := range cases {
		got, perm := c.p.Duration(c.c)
		if got != c.want || perm != c.perm {
			t.Errorf("%s: T_ban(%d) = %v, %v; want %v, %v", c.name, c.c, got, perm, c.want, c.perm)
		}
	}
}

func TestStepDecay(t *testing.T) {
	d := 7 * 24 * time.Hour
	t0 := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		n    int
		now  time.Time
		want int
	}{
		{"during a ban (quiet time not started)", 3, t0.Add(-time.Hour), 3},
		{"at the start", 3, t0, 3},
		{"just before one step", 3, t0.Add(d - time.Second), 3},
		{"exactly one step", 3, t0.Add(d), 2},
		{"several steps at once", 6, t0.Add(4*d + time.Hour), 2},
		{"floors at zero", 2, t0.Add(10 * d), 0},
		{"banned once is forgotten after D", 1, t0.Add(d), 0},
	}
	for _, c := range cases {
		if got := Effective(c.n, t0, c.now, d); got != c.want {
			t.Errorf("%s: %d, want %d", c.name, got, c.want)
		}
	}
	if got := NextDecrement(3, t0, t0.Add(d+time.Hour), d); !got.Equal(t0.Add(2 * d)) {
		t.Errorf("next decrement %v", got)
	}
	if got := NextDecrement(3, t0, t0.Add(-time.Hour), d); !got.Equal(t0.Add(d)) {
		t.Errorf("next decrement during a ban %v", got)
	}
	if got := NextDecrement(1, t0, t0.Add(d), d); !got.IsZero() {
		t.Errorf("next decrement at zero %v", got)
	}
}

func TestKey(t *testing.T) {
	for in, want := range map[string]string{
		"198.51.100.7":            "198.51.100.7",
		"::ffff:198.51.100.7":     "198.51.100.7",
		"2001:db8:1:2:3:4:5:6":    "2001:db8:1:2::/64",
		"2001:db8:1:2:ffff::1234": "2001:db8:1:2::/64",
	} {
		if got := KeyString(netip.MustParseAddr(in)); got != want {
			t.Errorf("%s → %s, want %s", in, got, want)
		}
	}
}
