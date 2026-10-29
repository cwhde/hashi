#!/bin/sh
# Test stand-in for systemd-run and systemctl in the firewall integration container (no systemd there).
# systemd-run --on-active=N ... --unit=NAME CMD: run CMD after N seconds in the background; systemctl stop NAME.timer
# cancels it. Everything else succeeds; is-enabled answers "enabled".
case "$(basename "$0")" in
systemd-run)
	for a in "$@"; do
		case "$a" in
		--on-active=*) secs=${a#--on-active=} ;;
		--unit=*) unit=${a#--unit=} ;;
		--*) ;;
		*) cmd=$a ;;
		esac
	done
	[ -e "/run/$unit.pid" ] && { echo "Unit $unit.timer was already loaded" >&2; exit 1; }
	(sleep "$secs"; rm -f "/run/$unit.pid"; sh "$cmd") >/dev/null 2>&1 &
	echo $! >"/run/$unit.pid"
	;;
systemctl)
	case "$1" in
	stop)
		unit=${2%.timer}
		[ -e "/run/$unit.pid" ] && kill "$(cat "/run/$unit.pid")" 2>/dev/null
		rm -f "/run/$unit.pid"
		;;
	is-enabled) echo enabled ;;
	esac
	;;
esac
exit 0
