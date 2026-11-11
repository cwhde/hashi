#!/bin/sh
# Firing times systemd computes for the OnCalendar expressions internal/scripts emits (read-only).
for e in 'Sun *-*-* 03:30:00' '*-*-* *:00,15,30,45:00' '*-*-01 04:00:00' 'Mon,Tue,Wed,Thu,Fri *-*-* 00:00:00' \
	'Mon *-*-* 12:00:00' '*-*-01 12:00:00' '*-*-* 00:00:00' '*-*-* 00,06,12,18:05,35:00' '*-01,06-* 09,13,17:00:00' \
	'Fri *-*-* 00:00:00' '*-*-13 00:00:00' 'Sun *-*-* 02:15:00'; do
	printf -- '-- %s\n' "$e"
	TZ=UTC systemd-analyze calendar --iterations=5 --base-time='2026-11-11 10:00:00 UTC' "$e" 2>&1 | grep -v 'From now'
	echo "rc=$?"
done
