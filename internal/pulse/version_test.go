package pulse

import "testing"

func TestVersionState(t *testing.T) {
	if VersionState(ReleaseTag) != VersionCurrent || VersionState("v0.0.1") != VersionOlder || VersionState("v99.0.0") != VersionNewer || VersionState("") != VersionUnknown {
		t.Fatal("states")
	}
}
