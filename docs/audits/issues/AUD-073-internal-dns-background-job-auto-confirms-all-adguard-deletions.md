# AUD-073: Internal DNS background job auto-confirms all AdGuard deletions

Priority: P1

Category: DNS / destructive confirmation

Status: open; confirmed by manual source tracing.

## Finding

SyncInternalAgentDnsAsync calls SyncManagedRewritesAsync(confirmDestructive:true) on the selected connection every maintenance interval. That method plans both topology and internal-agent rewrites, then applies the full plan, including stale generated deletions and duplicate cleanup. No human-approved plan is required. This bypasses the required destructive-change confirmation and broadens an internal DNS job to other rewrites.

## Evidence

Paths below are relative to src/Hashi.Infrastructure/:

Platform/SecurityAddendumJobWorker.cs:130-146; Platform/AdGuardSyncService.cs:180-195,295-331

## Expected outcome

Background reconciliation applies safe changes and records pending destructive plans for explicit approval; keep job workflow scope explicit.

## Acceptance criteria

Removing/renaming agents or changing topology leaves deletions pending confirmation; no background job supplies approval automatically.

No live service configuration or firewall was changed to establish this finding.
