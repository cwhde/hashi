// Package semver orders Hashi and Pulse release tags. It stays tiny: the Pulse agent uses it too.
package semver

import (
	"strconv"
	"strings"
)

// Compare orders two release tags (vMAJOR.MINOR.PATCH with an optional -pre.N, as semantic versioning does);
// ok is false when either is not one, such as a development build named by its commit.
func Compare(a, b string) (int, bool) {
	x, ok1 := parseVersion(a)
	y, ok2 := parseVersion(b)
	if !ok1 || !ok2 {
		return 0, false
	}
	for i := range 3 {
		if x.core[i] != y.core[i] {
			return cmpInt(x.core[i], y.core[i]), true
		}
	}
	switch { // a release is newer than its pre-releases
	case len(x.pre) == 0 && len(y.pre) == 0:
		return 0, true
	case len(x.pre) == 0:
		return 1, true
	case len(y.pre) == 0:
		return -1, true
	}
	for i := 0; i < len(x.pre) && i < len(y.pre); i++ {
		if c := cmpPart(x.pre[i], y.pre[i]); c != 0 {
			return c, true
		}
	}
	return cmpInt(len(x.pre), len(y.pre)), true
}

type version struct {
	core [3]int
	pre  []string
}

func parseVersion(s string) (version, bool) {
	var v version
	s, ok := strings.CutPrefix(s, "v")
	if !ok || isDescribe(s) {
		return v, false
	}
	core, pre, hasPre := strings.Cut(s, "-")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return v, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return v, false
		}
		v.core[i] = n
	}
	if hasPre {
		v.pre = strings.Split(pre, ".")
		for _, p := range v.pre {
			if p == "" {
				return v, false
			}
		}
	}
	return v, true
}

func cmpPart(a, b string) int {
	x, errA := strconv.Atoi(a)
	y, errB := strconv.Atoi(b)
	switch {
	case errA == nil && errB == nil:
		return cmpInt(x, y)
	case errA == nil:
		return -1
	case errB == nil:
		return 1
	}
	return strings.Compare(a, b)
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// isDescribe reports git describe's "-N-g<commit>[-dirty]" ending: a development build between two releases.
func isDescribe(s string) bool {
	s = strings.TrimSuffix(s, "-dirty")
	parts := strings.Split(s, "-")
	if len(parts) < 3 {
		return strings.HasSuffix(s, "-dirty")
	}
	g, n := parts[len(parts)-1], parts[len(parts)-2]
	if _, err := strconv.Atoi(n); err != nil || len(g) < 2 || g[0] != 'g' {
		return false
	}
	for _, c := range g[1:] {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return false
		}
	}
	return true
}
