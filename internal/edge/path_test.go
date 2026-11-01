package edge

import "testing"

func TestNormalizePath(t *testing.T) {
	for in, want := range map[string]string{
		"":                "/",
		"/":               "/",
		"/a?x=1":          "/a",
		"//a///b":         "/a/b",
		"/a/./b/../c":     "/a/c",
		"/%61dmin":        "/admin",
		"/%2e%2e/admin":   "/admin",
		"/x/%2E%2E/admin": "/admin",
		"/a%2fb":          "/a%2Fb",
		"/a/":             "/a/",
		"/../..":          "/",
		"/a/b/..":         "/a/",
		"/a%zz":           "/a%zz",
	} {
		if got := NormalizePath(in); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}
