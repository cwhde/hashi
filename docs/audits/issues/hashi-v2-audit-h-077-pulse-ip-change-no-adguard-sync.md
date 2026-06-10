# H-077: Pulse IP Change Does Not Trigger AdGuard Sync

**Priority:** Medium
**Conflict Type:** missing_implementation
**Spec Reference:** Main Spec §17.4, Addendum §9.4

**Status:** Fixed
**Branch:** audit-series-h

## Description

When a Pulse agent's IP changes, `AcceptHeartbeatAsync` triggers DNS provider sync via `ApplyDnsForPulseChangeAsync`, but does NOT trigger AdGuard topology rewrite sync. The AdGuard topology rewrites use the agent's last known IP, but the IP change doesn't automatically push updated rewrites to AdGuard. This means internal DNS rewrites can become stale until the next periodic AdGuard sync.

## Evidence

- `PulseAgentService.AcceptHeartbeatAsync` calls `ApplyDnsForPulseChangeAsync` for DNS providers
- Does not call `AdGuardSyncService.SyncResourceTopologyRewritesAsync` or any AdGuard sync method

## Expected Outcome

Pulse agent IP change should trigger both DNS provider sync AND AdGuard topology rewrite sync so internal rewrites are updated promptly.

## Fix Guidance

1. In `AcceptHeartbeatAsync`, after detecting an IP change, also queue an AdGuard sync for topology rewrites.
2. Call `AdGuardSyncService.SyncResourceTopologyRewritesAsync` or queue it via the sync orchestrator.

## Acceptance Criteria

- [ ] Pulse agent IP change triggers AdGuard topology rewrite sync
- [ ] Internal DNS rewrites update within seconds of IP change
- [ ] Both DNS provider and AdGuard rewrites are synced together
