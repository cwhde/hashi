# H-072: Entry Point Removal Has No Confirmation Step

**Priority:** Medium
**Conflict Type:** missing_implementation
**Spec Reference:** Main Spec §10.5

**Status:** Not Started
**Branch:** 

## Description

`TraefikEntryPointService.RemoveIfUnusedAsync` immediately removes the entry point when no enabled resources use that port. The spec requires confirmation before removing an entry point and associated firewall opening. Without confirmation, port closures happen silently when the last resource using that port is disabled or deleted.

## Evidence

- `TraefikEntryPointService.RemoveIfUnusedAsync` removes the entry point and port entity immediately
- No "pending removal" state or confirmation flow exists

## Expected Outcome

Removing the last resource using a public port should produce a pending change requiring user confirmation before the entry point and firewall opening are removed.

## Fix Guidance

1. Add a `ConfirmedForRemoval` boolean to `TraefikEntryPointEntity`.
2. When the last resource stops using a port, mark the entry point as pending removal rather than deleting it.
3. Show the pending removal in the sync plan.
4. Only remove after user confirmation via the sync apply flow.

## Acceptance Criteria

- [ ] Entry point removal requires user confirmation
- [ ] Pending entry point removals appear in sync plan
- [ ] Port remains open until confirmation is given
