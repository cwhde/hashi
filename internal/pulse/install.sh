#!/bin/sh
# Installs the Hashi Pulse agent (08 §1). Usage: install.sh --url <hashi> --id <agent> --token <token> [--interface <name>]
set -eu

url="" id="" token="" iface=""
while [ $# -gt 0 ]; do
	case "$1" in
	--url) url="$2"; shift 2 ;;
	--id) id="$2"; shift 2 ;;
	--token) token="$2"; shift 2 ;;
	--interface) iface="$2"; shift 2 ;;
	*) echo "unknown option: $1" >&2; exit 2 ;;
	esac
done
[ -n "$url" ] && [ -n "$id" ] && [ -n "$token" ] || { echo "--url, --id and --token are required" >&2; exit 2; }
[ "$(id -u)" = 0 ] || { echo "run as root" >&2; exit 1; }
for tool in curl sha256sum; do
	command -v "$tool" >/dev/null 2>&1 || { echo "this installer needs $tool" >&2; exit 1; }
done

case "$(uname -m)" in
x86_64 | amd64) arch=amd64 ;;
aarch64 | arm64) arch=arm64 ;;
*) echo "unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
curl -fsSL "$url/api/pulse/download/$arch" -o "$tmp/hashi-pulse"
want=$(curl -fsSL "$url/api/pulse/download/$arch.sha256")
got=$(sha256sum "$tmp/hashi-pulse" | cut -d' ' -f1)
[ "$want" = "$got" ] || { echo "checksum mismatch" >&2; exit 1; }
install -m 0755 "$tmp/hashi-pulse" /usr/local/bin/hashi-pulse

umask 077
mkdir -p /etc/hashi-pulse
{
	echo "url: $url"
	echo "agent_id: $id"
	echo "token: $token"
	[ -z "$iface" ] || echo "interface: $iface"
} >/etc/hashi-pulse/config.yaml
chmod 0600 /etc/hashi-pulse/config.yaml
umask 022

if command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
	cat >/etc/systemd/system/hashi-pulse.service <<'UNIT'
[Unit]
Description=Hashi Pulse agent
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=/usr/local/bin/hashi-pulse
Restart=always
RestartSec=10
ProtectSystem=strict
ProtectHome=yes
NoNewPrivileges=yes

[Install]
WantedBy=multi-user.target
UNIT
	systemctl daemon-reload
	systemctl enable hashi-pulse.service >/dev/null
	systemctl restart hashi-pulse.service
	echo "Hashi Pulse is running (systemd)."
elif command -v rc-service >/dev/null 2>&1; then
	cat >/etc/init.d/hashi-pulse <<'INIT'
#!/sbin/openrc-run
name="Hashi Pulse"
supervisor=supervise-daemon
command=/usr/local/bin/hashi-pulse
depend() { need net; }
INIT
	chmod 0755 /etc/init.d/hashi-pulse
	rc-update add hashi-pulse default >/dev/null
	rc-service hashi-pulse restart >/dev/null
	echo "Hashi Pulse is running (OpenRC)."
else
	line="@reboot /usr/local/bin/hashi-pulse >/dev/null 2>&1"
	(crontab -l 2>/dev/null | grep -vF hashi-pulse; echo "$line") | crontab -
	nohup /usr/local/bin/hashi-pulse >/dev/null 2>&1 &
	echo "Hashi Pulse is running (cron)."
fi
