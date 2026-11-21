#!/bin/sh
# Installs the Hashi Pulse agent (08 §1). Usage: install.sh --url <hashi> --id <agent> --token <token> [--interface <name>]
# Hashi serves this script; the binary comes from the Gitea release named below, or from --from <base URL>.
set -eu

url="" id="" token="" iface="" from="@@RELEASE_BASE@@"
while [ $# -gt 0 ]; do
	case "$1" in
	--url) url="$2"; shift 2 ;;
	--id) id="$2"; shift 2 ;;
	--token) token="$2"; shift 2 ;;
	--interface) iface="$2"; shift 2 ;;
	--from) from="$2"; shift 2 ;;
	*) echo "unknown option: $1" >&2; exit 2 ;;
	esac
done
if [ -z "$url" ] || [ -z "$id" ] || [ -z "$token" ]; then
	echo "--url, --id and --token are required" >&2
	exit 2
fi
[ "$(id -u)" = 0 ] || { echo "run as root" >&2; exit 1; }
command -v sha256sum >/dev/null 2>&1 || { echo "this installer needs sha256sum" >&2; exit 1; }
# curl where it exists; OpenWrt has only wget (uclient-fetch), which takes -q and -O.
if command -v curl >/dev/null 2>&1; then
	fetch() { curl -fsSL "$1" -o "$2"; }
elif command -v wget >/dev/null 2>&1; then
	fetch() { wget -q -O "$2" "$1"; }
else
	echo "this installer needs curl or wget" >&2
	exit 1
fi

case "$(uname -m)" in
x86_64 | amd64) arch=amd64 ;;
aarch64 | arm64) arch=arm64 ;;
*) echo "unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
fetch "$from/hashi-pulse-linux-$arch" "$tmp/hashi-pulse"
fetch "$from/hashi-pulse-linux-$arch.sha256" "$tmp/sum"
want=$(cat "$tmp/sum")
got=$(sha256sum "$tmp/hashi-pulse" | cut -d' ' -f1)
[ "$want" = "$got" ] || { echo "checksum mismatch" >&2; exit 1; }
# Not install(1), which OpenWrt lacks. Renamed into place, so a running agent's file is replaced, not written into.
mkdir -p /usr/local/bin
cp "$tmp/hashi-pulse" /usr/local/bin/hashi-pulse.new
chmod 0755 /usr/local/bin/hashi-pulse.new
mv -f /usr/local/bin/hashi-pulse.new /usr/local/bin/hashi-pulse

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
# The agent replaces its own file when it updates itself (ADR 0003).
ReadWritePaths=/usr/local/bin
NoNewPrivileges=yes

[Install]
WantedBy=multi-user.target
UNIT
	systemctl daemon-reload
	systemctl enable hashi-pulse.service >/dev/null
	systemctl restart hashi-pulse.service
	echo "Hashi Pulse is running (systemd)."
elif [ -x /sbin/procd ] && [ -f /etc/rc.common ]; then
	# OpenWrt: a procd service, and a keep list so a firmware upgrade keeps the agent (sysupgrade keeps only listed files).
	cat >/etc/init.d/hashi-pulse <<'INIT'
#!/bin/sh /etc/rc.common
START=99
STOP=10
USE_PROCD=1

start_service() {
	procd_open_instance
	procd_set_param command /usr/local/bin/hashi-pulse
	procd_set_param respawn
	procd_set_param stdout 1
	procd_set_param stderr 1
	procd_close_instance
}
INIT
	chmod 0755 /etc/init.d/hashi-pulse
	mkdir -p /lib/upgrade/keep.d
	cat >/lib/upgrade/keep.d/hashi-pulse <<'KEEP'
/usr/local/bin/hashi-pulse
/etc/hashi-pulse/
/etc/init.d/hashi-pulse
/etc/rc.d/S99hashi-pulse
/lib/upgrade/keep.d/hashi-pulse
KEEP
	/etc/init.d/hashi-pulse enable
	# restart on a first install makes procd report the missing service as a failure
	if /etc/init.d/hashi-pulse running; then /etc/init.d/hashi-pulse restart; else /etc/init.d/hashi-pulse start; fi
	echo "Hashi Pulse is running (procd)."
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
