package services

import (
	"slices"
	"testing"

	"git.juzo.io/juzo/hashi/internal/settings"
)

func TestResolveWAF(t *testing.T) {
	w := WAF{Exclusions: []int{942100}, Paths: []WAFPath{
		{Match: MatchPrefix, Path: "/remote.php", Uploads: true, Exclusions: []int{920230}},
		{Match: MatchPrefix, Path: "/remote.php/dav/raw", Mode: settings.WAFDisabled},
		{Match: MatchExact, Path: "/api/raw", Mode: settings.WAFDisabled},
		{Match: MatchPrefix, Path: "/api", Mode: settings.WAFHeadersOnly},
	}}
	cases := []struct {
		path    string
		mode    string
		uploads bool
		excl    []int
	}{
		{"/", settings.WAFDisabled, false, []int{942100}},
		{"/remote.php/dav/files", settings.WAFDisabled, true, []int{920230, 942100}},
		{"/remote.php/dav/raw/x", settings.WAFDisabled, false, []int{942100}},
		{"/api/raw", settings.WAFDisabled, false, []int{942100}},
		{"/api/raw/x", settings.WAFHeadersOnly, false, []int{942100}},
	}
	for _, c := range cases {
		got := ResolveWAF(w, false, settings.WAFDisabled, c.path)
		if got.Mode != c.mode || got.Uploads != c.uploads || !slices.Equal(got.Exclusions, c.excl) {
			t.Errorf("%s: %+v", c.path, got)
		}
	}
	if got := ResolveWAF(w, true, settings.WAFHeadersOnly, "/api/raw"); got.Mode != settings.WAFHeadersOnly || !got.Uploads {
		t.Errorf("system service: %+v", got)
	}
	w.Mode = settings.WAFHeadersOnly
	if got := ResolveWAF(w, false, settings.WAFDisabled, "/"); got.Mode != settings.WAFHeadersOnly {
		t.Errorf("service mode over the default: %+v", got)
	}
}
