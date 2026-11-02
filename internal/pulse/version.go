package pulse

import "git.juzo.io/juzo/hashi/internal/semver"

// Version states of an agent against the one this Hashi installs (08 §6).
const (
	VersionCurrent = "current"
	VersionOlder   = "older"
	VersionNewer   = "newer"
	VersionUnknown = "unknown" // no heartbeat yet, or a development build
)

// VersionState compares an agent's reported version with ReleaseTag.
func VersionState(agent string) string {
	c, ok := semver.Compare(agent, ReleaseTag)
	switch {
	case !ok:
		return VersionUnknown
	case c < 0:
		return VersionOlder
	case c > 0:
		return VersionNewer
	}
	return VersionCurrent
}
