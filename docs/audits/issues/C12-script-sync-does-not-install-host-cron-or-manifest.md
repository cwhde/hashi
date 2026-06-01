# C12 - Script sync does not install host cron or manifest

Priority: High

Spec conflicts: section 23 requires scripts to be written to `/opt/hashi/scripts`, root-owned, with a manifest of script hashes and cron entries in `/etc/cron.d/hashi-scripts` or systemd timers. Target hosts default to all Linux firewall hosts.

## Problem

Scripts are copied to `/opt/hashi/scripts` with hardened ownership and mode, but the rest of the sync behavior is centralized inside the Hashi process. `ScriptCronHostedService` wakes every minute, deploys enabled scripts, checks cron expressions in the database, and executes due scripts over SSH. It does not install host cron entries or systemd timers.

Hashi also does not write a remote manifest with script hashes, so target hosts do not have a durable view of what Hashi intended to install. When a script has no explicit target rows, execution defaults to `script.ConnectionId`, not all Linux firewall hosts.

## Evidence

- `docs/implementation-spec/hashi-v2-implementation-spec.md:1220-1243` defines script fields, sync behavior, manifest, cron/timer generation, and default target hosts.
- `src/Hashi.Infrastructure/Platform/ScriptExecutionService.cs:21` sets the script directory to `/opt/hashi/scripts`.
- `src/Hashi.Infrastructure/Platform/ScriptExecutionService.cs:177-197` deploys enabled scripts but does not write cron/timer files or a manifest.
- `src/Hashi.Infrastructure/Platform/ScriptExecutionService.cs:237-287` writes and hardens the script file only.
- `src/Hashi.Infrastructure/Platform/ScriptCronHostedService.cs:23-40` runs cron scheduling centrally inside Hashi and executes due scripts over SSH.
- `src/Hashi.Infrastructure/Platform/ScriptCronHostedService.cs:60-66` records the hosted worker run as "Synced scripts; executed ... due cron run(s)."
- `src/Hashi.Infrastructure/Platform/ScriptExecutionService.cs:468-475` defaults missing target rows to `script.ConnectionId`, not all Linux firewall hosts.
- `rg -n "/etc/cron\\.d/hashi-scripts|hashi-scripts|manifest|hashes" src tests` finds no host script cron/manifest implementation.

## Expected outcome

Script sync should install the remote script files, a manifest with hashes, and remote cron/timer definitions on target Linux firewall hosts. Hashi should still support manual SSH runs, but scheduled execution should not depend only on the Hashi process waking and initiating SSH.

## Fix guidance

Render a host-side manifest that records script ids, paths, hashes, enabled state, and cron/timer metadata. Render `/etc/cron.d/hashi-scripts` or systemd timer units per target host and apply them through the same preview/apply/audit pattern. Default target selection should expand to all enabled firewall-host connections when no explicit targets are configured.

## Acceptance criteria

- Script sync writes a remote manifest with hashes for deployed scripts.
- Script sync creates or updates `/etc/cron.d/hashi-scripts` or systemd timers on target hosts.
- Removing or disabling a script updates the manifest and removes or disables its scheduled host entry.
- Scripts with no explicit targets default to all enabled Linux firewall hosts.
- Tests cover manifest rendering, cron/timer rendering, target default expansion, and stale scheduled entry cleanup.
