package pulse

import (
	"strings"
	"testing"
)

// The agent image carries the Hashi image's tag, never a floating one.
func TestImageMatchesRelease(t *testing.T) {
	if got := Image(); !strings.HasSuffix(got, ":"+strings.TrimPrefix(ReleaseTag, "v")) {
		t.Fatal(got)
	}
}
