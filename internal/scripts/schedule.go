// Package scripts manages the shell scripts Hashi keeps on firewall hosts (09 §1).
package scripts

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Schedule is a parsed five-field cron expression: minute, hour, day of month, month, day of week.
type Schedule struct {
	expr                       string
	min, hour, dom, month, dow []int // sorted values; dow 0 = Sunday
	domAny, dowAny, monthAny   bool
	hourAny, minAny            bool
}

var macros = map[string]string{
	"@hourly": "0 * * * *", "@daily": "0 0 * * *", "@midnight": "0 0 * * *",
	"@weekly": "0 0 * * 0", "@monthly": "0 0 1 * *", "@yearly": "0 0 1 1 *", "@annually": "0 0 1 1 *",
}

// ParseCron reads a five-field cron expression (numbers, `*`, lists, ranges, steps) or one of the @ macros.
func ParseCron(expr string) (Schedule, error) {
	s := Schedule{expr: strings.TrimSpace(expr)}
	e := s.expr
	if m, ok := macros[e]; ok {
		e = m
	}
	f := strings.Fields(e)
	if len(f) != 5 {
		return s, fmt.Errorf("a schedule has five fields (minute hour day month weekday), got %d", len(f))
	}
	var err error
	if s.min, s.minAny, err = field(f[0], 0, 59, "minute"); err != nil {
		return s, err
	}
	if s.hour, s.hourAny, err = field(f[1], 0, 23, "hour"); err != nil {
		return s, err
	}
	if s.dom, s.domAny, err = field(f[2], 1, 31, "day of month"); err != nil {
		return s, err
	}
	if s.month, s.monthAny, err = field(f[3], 1, 12, "month"); err != nil {
		return s, err
	}
	if s.dow, s.dowAny, err = field(f[4], 0, 7, "weekday"); err != nil {
		return s, err
	}
	for i, d := range s.dow { // 7 is Sunday too
		if d == 7 {
			s.dow[i] = 0
		}
	}
	slices.Sort(s.dow)
	s.dow = slices.Compact(s.dow)
	return s, nil
}

// field parses one cron field into its sorted values; any reports a bare `*`.
func field(f string, lo, hi int, name string) ([]int, bool, error) {
	if f == "*" {
		return span(lo, hi, 1), true, nil
	}
	var out []int
	for _, part := range strings.Split(f, ",") {
		rng, step := part, 1
		if i := strings.IndexByte(part, '/'); i >= 0 {
			n, err := strconv.Atoi(part[i+1:])
			if err != nil || n < 1 {
				return nil, false, fmt.Errorf("%s: bad step in %q", name, part)
			}
			rng, step = part[:i], n
		}
		a, b := lo, hi
		switch {
		case rng == "*":
		case strings.Contains(rng, "-"):
			x, y, _ := strings.Cut(rng, "-")
			var err1, err2 error
			a, err1 = strconv.Atoi(x)
			b, err2 = strconv.Atoi(y)
			if err1 != nil || err2 != nil || a > b {
				return nil, false, fmt.Errorf("%s: bad range %q", name, rng)
			}
		default:
			n, err := strconv.Atoi(rng)
			if err != nil {
				return nil, false, fmt.Errorf("%s: %q is not a number", name, rng)
			}
			a, b = n, n
			if step > 1 { // "5/15" means from 5 to the end
				b = hi
			}
		}
		if a < lo || b > hi {
			return nil, false, fmt.Errorf("%s: %q is outside %d-%d", name, part, lo, hi)
		}
		out = append(out, span(a, b, step)...)
	}
	slices.Sort(out)
	return slices.Compact(out), false, nil
}

func span(a, b, step int) []int {
	var out []int
	for v := a; v <= b; v += step {
		out = append(out, v)
	}
	return out
}

var dayNames = [...]string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}

// OnCalendar converts the schedule into systemd timer expressions. A timer fires on the union of its OnCalendar lines.
// Cron fires when the day of month OR the weekday matches if both are restricted; systemd requires both
// (testdata/contracts/scripts, finding 2), so that case becomes two lines.
func (s Schedule) OnCalendar() []string {
	clock := list(s.hour, s.hourAny) + ":" + list(s.min, s.minAny) + ":00"
	date := func(dom string) string { return "*-" + list(s.month, s.monthAny) + "-" + dom }
	days := func() string {
		names := make([]string, len(s.dow))
		for i, d := range s.dow {
			names[i] = dayNames[d]
		}
		return strings.Join(names, ",")
	}
	dom := list(s.dom, s.domAny)
	switch {
	case s.dowAny:
		return []string{date(dom) + " " + clock}
	case s.domAny:
		return []string{days() + " " + date("*") + " " + clock}
	}
	return []string{days() + " " + date("*") + " " + clock, date(dom) + " " + clock}
}

func list(v []int, any bool) string {
	if any {
		return "*"
	}
	parts := make([]string, len(v))
	for i, n := range v {
		parts[i] = fmt.Sprintf("%02d", n)
	}
	return strings.Join(parts, ",")
}

// Next returns the first n firing times after t, in t's location, with cron's own rules. It backs the schedule
// preview and the round-trip test against systemd.
func (s Schedule) Next(t time.Time, n int) []time.Time {
	var out []time.Time
	c := t.Truncate(time.Minute).Add(time.Minute)
	for limit := c.AddDate(5, 0, 0); len(out) < n && c.Before(limit); c = c.Add(time.Minute) {
		if s.matches(c) {
			out = append(out, c)
		}
	}
	return out
}

func (s Schedule) matches(t time.Time) bool {
	if !slices.Contains(s.min, t.Minute()) || !slices.Contains(s.hour, t.Hour()) || !slices.Contains(s.month, int(t.Month())) {
		return false
	}
	domOK, dowOK := slices.Contains(s.dom, t.Day()), slices.Contains(s.dow, int(t.Weekday()))
	if !s.domAny && !s.dowAny {
		return domOK || dowOK
	}
	return domOK && dowOK
}

// String is the expression as written.
func (s Schedule) String() string { return s.expr }
