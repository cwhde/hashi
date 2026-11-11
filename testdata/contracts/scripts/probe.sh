#!/bin/sh
# Contract probe for 09 §1 (scripts): systemd calendar parsing, a timer + service pair, the timeout wrapper's exit codes,
# file modes. Dev targets only: it writes units under /etc/systemd/system and files under /opt/hashi-probe, and removes them.
set -u
section() { printf '\n### %s\n' "$*"; }
section versions
systemctl --version | head -1
timeout --version | head -1
section "calendar: fixed base time"
for e in 'Sun *-*-* 03:30:00' '*-*-* *:00/15:00' '*-*-01 04:00:00' 'Mon..Fri *-*-* 00:00:00' 'Mon *-*-01 12:00:00' \
	'*-*-* 03:30:00' 'bogus'; do
	printf -- '-- %s\n' "$e"
	systemd-analyze calendar --iterations=3 --base-time='2026-11-11 10:00:00 UTC' "$e" 2>&1
	echo "rc=$?"
done
section "unit files"
d=/opt/hashi-probe; rm -rf "$d"; install -d -m 0750 -o root -g root "$d"
printf '#!/bin/sh\necho out; echo err >&2; exit "${1:-0}"\n' > "$d/ok.sh"; chmod 0750 "$d/ok.sh"
printf '#!/bin/sh\nsleep 30\n' > "$d/slow.sh"; chmod 0750 "$d/slow.sh"
umask 077; printf 'KEEP=14\n' > "$d/ok.env"; umask 022
stat -c '%a %U:%G %n' "$d" "$d/ok.sh" "$d/ok.env"
cat > /etc/systemd/system/hashi-probe.service <<UNIT
[Unit]
Description=Hashi probe
[Service]
Type=oneshot
EnvironmentFile=$d/ok.env
ExecStart=$d/ok.sh 3
UNIT
cat > /etc/systemd/system/hashi-probe.timer <<UNIT
[Unit]
Description=Hashi probe timer
[Timer]
OnCalendar=*-*-* 03:30:00
Persistent=true
[Install]
WantedBy=timers.target
UNIT
systemctl daemon-reload
systemctl start hashi-probe.timer; echo "start timer rc=$?"
section "list-timers json"
systemctl list-timers --all --output=json hashi-probe.timer
echo
section "timer show"
systemctl show hashi-probe.timer -p NextElapseUSecRealtime -p LastTriggerUSec -p ActiveState -p Result
section "service run (exit 3)"
systemctl start hashi-probe.service; echo "start rc=$?"
systemctl show hashi-probe.service -p Result -p ExecMainStatus -p ExecMainCode -p ActiveState -p ExecMainStartTimestamp -p ExecMainExitTimestamp
section "timeout wrapper"
timeout 2 "$d/slow.sh"; echo "timeout rc=$?"
timeout -k 1 2 "$d/slow.sh"; echo "timeout -k rc=$?"
"$d/ok.sh" 0 >/dev/null 2>&1; echo "ok rc=$?"
section cleanup
systemctl stop hashi-probe.timer hashi-probe.service 2>/dev/null
systemctl reset-failed hashi-probe.service 2>/dev/null
rm -f /etc/systemd/system/hashi-probe.service /etc/systemd/system/hashi-probe.timer
systemctl daemon-reload; rm -rf "$d"
systemctl list-timers --all --output=json hashi-probe.timer; echo
echo done
