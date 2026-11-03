package traefik

import (
	"fmt"
	"strings"
)

// ServiceName is Hashi's Traefik service on every host. It differs from a distribution's "traefik" service so an
// existing installation is never overwritten by name (04 §2 steps 4–5).
const ServiceName = "hashi-traefik"

// Service is the init-system definition and the commands that drive it. Commands are fixed strings: nothing in them
// comes from user input.
type Service struct {
	Path            string
	Data            []byte
	Mode            uint32
	Enable, Restart string
	// Running exits 0 when the service is up.
	Running string
}

// ServiceFor returns the OpenRC script or systemd unit for init. Traefik runs as root in S4; a dedicated user is a
// later hardening step (it needs per-distribution user creation).
func ServiceFor(init string, paths Paths) (Service, error) {
	run := paths.Binary + " --configFile=" + paths.Static()
	switch init {
	case InitOpenRC:
		// Shape of Alpine's own traefik script (testdata/contracts/os/openrc_traefik_alpine.txt).
		script := strings.Join([]string{
			"#!/sbin/openrc-run",
			"# Managed by Hashi. Changes are overwritten.",
			"supervisor=supervise-daemon",
			"",
			`name="Traefik (Hashi)"`,
			`description="Traefik reverse proxy managed by Hashi"`,
			"",
			`command="` + paths.Binary + `"`,
			`command_args="--configFile=` + paths.Static() + `"`,
			"",
			"healthcheck_timer=30",
			"",
			"depend() {",
			"\tneed net",
			"\tafter firewall",
			"}",
			"",
			"healthcheck() {",
			"\t$command healthcheck $command_args >/dev/null",
			"}",
			"",
		}, "\n")
		return Service{
			Path: "/etc/init.d/" + ServiceName, Data: []byte(script), Mode: 0o755,
			Enable:  "rc-update add " + ServiceName + " default && rc-service " + ServiceName + " start",
			Restart: "rc-service " + ServiceName + " restart",
			Running: "rc-service " + ServiceName + " status",
		}, nil
	case InitSystemd:
		unit := strings.Join([]string{
			"# Managed by Hashi. Changes are overwritten.",
			"[Unit]",
			"Description=Traefik reverse proxy managed by Hashi",
			"After=network-online.target",
			"Wants=network-online.target",
			"",
			"[Service]",
			"ExecStart=" + run,
			"Restart=on-failure",
			"RestartSec=2",
			"LimitNOFILE=1048576",
			"",
			"[Install]",
			"WantedBy=multi-user.target",
			"",
		}, "\n")
		return Service{
			Path: "/etc/systemd/system/" + ServiceName + ".service", Data: []byte(unit), Mode: 0o644,
			Enable:  "systemctl daemon-reload && systemctl enable --now " + ServiceName,
			Restart: "systemctl daemon-reload && systemctl restart " + ServiceName,
			Running: "systemctl is-active --quiet " + ServiceName,
		}, nil
	}
	return Service{}, fmt.Errorf("%w: init system %q", ErrUnsupported, init)
}
