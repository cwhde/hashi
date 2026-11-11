package pulse

import (
	"bytes"
)

// ReleaseTag names the Gitea release whose agent binaries installs use (08 §1, §6). The release job refuses a tag that
// differs from it, so a release and the Hashi built with it always agree.
const ReleaseTag = "v0.11.0-beta.1"

// Image is the agent image for Compose: latest, which the release moves on every version tag, so an image updater keeps
// Docker agents current (ADR 0003).
func Image() string { return "git.juzo.io/juzo/hashi-pulse:latest" }

// ReleaseBase is where that release's files are.
const ReleaseBase = "https://git.juzo.io/juzo/hashi/releases/download/" + ReleaseTag

// Script is the install script with this version's release as its download source.
func Script() []byte {
	return bytes.ReplaceAll(InstallScript, []byte("@@RELEASE_BASE@@"), []byte(ReleaseBase))
}
