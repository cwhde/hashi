package traefik

import (
	"fmt"

	"git.juzo.io/juzo/hashi/internal/sshx"
)

// LogrotatePath is Hashi's logrotate config for Traefik's logs (04 §3). Every supported OS includes /etc/logrotate.d
// from /etc/logrotate.conf and runs logrotate daily (testdata/contracts/os/README.md).
const LogrotatePath = "/etc/logrotate.d/hashi-traefik"

// logrotateFile rotates the access and Traefik logs daily (and at 100 MB), keeps 14, and asks Hashi's Traefik (matched by
// its exact command line, so no other Traefik on the host) to reopen its files with USR1.
func logrotateFile(p Paths) File {
	pattern := "^" + p.Binary + " --configFile=" + p.Static() + "$"
	data := fmt.Sprintf(`# Managed by Hashi. Changes are overwritten.
%s/*.log {
	daily
	maxsize 100M
	rotate 14
	missingok
	notifempty
	compress
	delaycompress
	dateext
	sharedscripts
	postrotate
		pkill -USR1 -f %s || true
	endscript
}
`, LogDir, sshx.Quote(pattern))
	return File{Path: LogrotatePath, Kind: KindSystem, Mode: 0o644, Data: []byte(data)}
}

// logrotateInstall installs logrotate with the host's package manager (captured on Alpine, Debian and Fedora); on
// systemd hosts its timer is enabled and started (Fedora's package only enables it).
func logrotateInstall(pm, init string) (string, error) {
	var cmd string
	switch pm {
	case "apk":
		cmd = "apk add --no-cache logrotate"
	case "apt-get":
		// The package index may be empty (fresh images): refresh it first (debian:13, ubuntu:24.04 captured).
		cmd = "apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y logrotate"
	case "dnf":
		cmd = "dnf install -y logrotate"
	default:
		return "", fmt.Errorf("%w: no way to install logrotate with %q", ErrUnsupported, pm)
	}
	if init == InitSystemd {
		cmd += " && systemctl enable --now logrotate.timer"
	}
	return cmd, nil
}
