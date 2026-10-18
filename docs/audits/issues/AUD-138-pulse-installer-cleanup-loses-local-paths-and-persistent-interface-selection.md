# AUD-138: Pulse installer cleanup loses local paths and persistent interface selection

Priority: P2

Category: Installer reliability / Pulse

Status: open; confirmed by manual source trace.

## Finding

install_binary installs a global EXIT trap referring to function-local tmp_binary/tmp_checksum under set -u. After the function returns those names are unset; a successful later exit triggers an unbound-variable error and temporary downloads are not removed. It also computes the release base before honoring BINARY_URL, so a direct artifact override still depends on the latest-release API unless a version/base override is supplied. write_config persists API/ID/token/interval but omits HASHI_PULSE_INTERFACE, so a selected installation-time interface is lost by the service runner. Switching installation modes leaves the previous cron/timer active.

## Evidence

agents/pulse/install.sh:144-156,189-214,219-274; cmd/hashi-pulse/main.go:88. These are separate from AUD-059, which prevents public retrieval of the installer itself.

## Expected outcome and acceptance criteria

Capture cleanup paths safely for the trap and remove it after successful cleanup; honor direct binary overrides without unnecessary discovery, persist supported address selectors, and disable the old scheduler when switching modes. Verify the installer in a disposable filesystem/service harness, not on a live host.
