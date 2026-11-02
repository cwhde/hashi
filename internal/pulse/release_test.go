package pulse

import (
	"strings"
	"testing"
)

// Docker agents follow the newest release through :latest, which moves only with version tags (ADR 0003).
func TestImageIsLatest(t *testing.T) {
	if got := Image(); !strings.HasSuffix(got, "/hashi-pulse:latest") {
		t.Fatal(got)
	}
}
