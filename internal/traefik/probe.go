// Package traefik installs and configures Traefik on edge hosts over SSH (04-traefik-services.md §2–6): it reads the
// host's OS, renders Hashi's static and dynamic configuration as YAML from Go structs, and describes the install items
// (pinned release binary, directories, service definition).
package traefik

import (
	"bufio"
	"errors"
	"fmt"
	"path"
	"slices"
	"strconv"
	"strings"
)

// ProbeCommand is the read-only command validation runs on a host. Its output format is the contract in
// testdata/contracts/os/: os-release, then sections introduced by "--- <name>" lines.
const ProbeCommand = `cat /etc/os-release; echo "--- uname"; uname -m; echo "--- id"; id -u; ` +
	`echo "--- pm"; for c in apk apt-get dnf yum; do command -v $c; done; ` +
	`echo "--- init"; for c in systemctl rc-service openrc-run; do command -v $c; done; ` +
	`echo "--- traefik"; command -v traefik; ls -d /etc/traefik /opt/traefik 2>/dev/null; echo "--- end"`

// OS families Hashi installs on. Ubuntu and other apt-get systems share Debian's path, other dnf systems Fedora's.
const (
	FamilyAlpine = "alpine"
	FamilyDebian = "debian"
	FamilyFedora = "fedora"
)

// Init systems.
const (
	InitOpenRC  = "openrc"
	InitSystemd = "systemd"
)

// Probe is what Hashi learned about a host.
type Probe struct {
	ID, PrettyName  string // from os-release, shown to the user only
	Arch            string // Go/Traefik release arch (amd64, arm64, ...)
	Root            bool
	PackageManager  string // apk, apt-get, dnf
	Init            string // openrc, systemd
	Family          string
	ExistingBinary  string   // a traefik on PATH, "" when none
	ExistingConfigs []string // config directories found in common places
}

// ErrUnsupported means the host lacks what Hashi needs (package manager and init system it knows).
var ErrUnsupported = errors.New("unsupported host")

// archs maps `uname -m` to the arch in Traefik's release asset names (release/*.sha256).
var archs = map[string]string{
	"x86_64": "amd64", "aarch64": "arm64", "arm64": "arm64", "armv7l": "armv7", "armv6l": "armv6",
	"i386": "386", "i686": "386", "ppc64le": "ppc64le", "riscv64": "riscv64", "s390x": "s390x",
}

// ParseProbe reads ProbeCommand's output. The family is decided by the tools present, never by a version
// (04 §2): yum next to dnf is dnf's alias, so dnf wins.
func ParseProbe(out string) (Probe, error) {
	var p Probe
	osr := map[string]string{}
	section := "os-release"
	var pms, inits []string
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if name, ok := strings.CutPrefix(line, "--- "); ok {
			section = name
			continue
		}
		if line == "" {
			continue
		}
		switch section {
		case "os-release":
			if k, v, ok := strings.Cut(line, "="); ok && !strings.HasPrefix(k, "#") {
				osr[k] = unquote(v)
			}
		case "uname":
			p.Arch = archs[line]
			if p.Arch == "" {
				return p, fmt.Errorf("%w: CPU architecture %q", ErrUnsupported, line)
			}
		case "id":
			uid, err := strconv.Atoi(line)
			if err != nil {
				return p, fmt.Errorf("probe: user id %q", line)
			}
			p.Root = uid == 0
		case "pm":
			pms = append(pms, path.Base(line))
		case "init":
			inits = append(inits, path.Base(line))
		case "traefik":
			if strings.HasSuffix(line, "/traefik") && !strings.HasPrefix(line, "/etc/") && !strings.HasPrefix(line, "/opt/") {
				p.ExistingBinary = line
			} else {
				p.ExistingConfigs = append(p.ExistingConfigs, line)
			}
		}
	}
	if section != "end" {
		return p, errors.New("probe output is incomplete (no end marker)")
	}
	p.ID, p.PrettyName = osr["ID"], osr["PRETTY_NAME"]
	switch {
	case slices.Contains(pms, "apk"):
		p.PackageManager, p.Family = "apk", FamilyAlpine
	case slices.Contains(pms, "apt-get"):
		p.PackageManager, p.Family = "apt-get", FamilyDebian
	case slices.Contains(pms, "dnf"):
		p.PackageManager, p.Family = "dnf", FamilyFedora
	default:
		return p, fmt.Errorf("%w: no apk, apt-get or dnf found (%s)", ErrUnsupported, p.PrettyName)
	}
	switch {
	case slices.Contains(inits, "systemctl"):
		p.Init = InitSystemd
	case slices.Contains(inits, "openrc-run") && slices.Contains(inits, "rc-service"):
		p.Init = InitOpenRC
	default:
		return p, fmt.Errorf("%w: neither systemd nor OpenRC found (%s)", ErrUnsupported, p.PrettyName)
	}
	return p, nil
}

// unquote strips os-release's shell-style quoting (testdata/contracts/os/README.md).
func unquote(v string) string {
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
		v = v[1 : len(v)-1]
	}
	return strings.NewReplacer(`\"`, `"`, `\$`, `$`, "\\`", "`", `\\`, `\`).Replace(v)
}
