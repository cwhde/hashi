package scripts

import "testing"

// Bug 52: a run stopped by its time limit keeps what it printed and ends with a line saying it was stopped.
func TestStoppedNote(t *testing.T) {
	for _, c := range []struct {
		out  string
		code int
		want string
	}{
		{"partial", TimedOut, "partial\nStopped after 30 s (time limit)."},
		{"line\n", TimedOut, "line\nStopped after 30 s (time limit)."},
		{"", TimedOut, "Stopped after 30 s (time limit)."},
		{"done", 0, "done"},
		{"oops", 3, "oops"},
	} {
		if got := stopped(c.out, c.code, 30); got != c.want {
			t.Errorf("stopped(%q, %d) = %q, want %q", c.out, c.code, got, c.want)
		}
	}
}
