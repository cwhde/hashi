package pulse

import (
	"bytes"
	"strings"
)

// ReleaseTag names the Gitea release whose agent binaries installs use (08 §1, §6). The release job refuses a tag that
// differs from it, so a release and the Hashi built with it always agree.
const ReleaseTag = "v0.8.0-beta.3"

// Image is the agent image of that release: it carries the same tag as the Hashi image, without the v.
func Image() string { return "git.juzo.io/juzo/hashi-pulse:" + strings.TrimPrefix(ReleaseTag, "v") }

// ReleaseBase is where that release's files are.
const ReleaseBase = "https://git.juzo.io/juzo/hashi/releases/download/" + ReleaseTag

// Script is the install script with this version's release as its download source.
func Script() []byte {
	return bytes.ReplaceAll(InstallScript, []byte("@@RELEASE_BASE@@"), []byte(ReleaseBase))
}
