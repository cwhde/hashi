package semver

import "testing"

func TestCompare(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
		ok   bool
	}{
		{"v0.9.2", "v0.9.2", 0, true},
		{"v0.9.1", "v0.9.2", -1, true},
		{"v0.10.0", "v0.9.9", 1, true},
		{"v0.9.1-beta.2", "v0.9.1", -1, true},
		{"v0.9.1-beta.10", "v0.9.1-beta.2", 1, true},
		{"v0.9.1-beta", "v0.9.1-beta.1", -1, true},
		{"v0.8.0-beta.3", "v0.9.1-beta.2", -1, true},
		{"dev", "v0.9.2", 0, false},
		{"v0.9.1-7-gcc8a0c9", "v0.9.2", 0, false},
		{"v0.9.1-7-gcc8a0c9-dirty", "v0.9.2", 0, false},
		{"v0.9", "v0.9.2", 0, false},
		{"", "v0.9.2", 0, false},
	} {
		got, ok := Compare(c.a, c.b)
		if got != c.want || ok != c.ok {
			t.Errorf("%s vs %s: %d %v", c.a, c.b, got, ok)
		}
	}
}
