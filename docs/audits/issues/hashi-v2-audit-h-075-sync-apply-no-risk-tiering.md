# H-075: Sync Apply Has No Risk Tiering

**Priority:** Medium
**Conflict Type:** wrong_implementation
**Spec Reference:** Main Spec §25

**Status:** In Progress
**Branch:** h/sync-engine
**Branch:** 

## Description

`ApplyGlobalAsync()` takes a single `confirmDestructive` boolean. It doesn't separate low-risk auto-apply from high-risk confirmation — the entire Apply is all-or-nothing. If `confirmDestructive=false` and ANY subsystem has destructive changes, the entire Apply aborts for that subsystem. The spec requires auto-applying safe changes while only holding destructive changes for confirmation.

## Evidence

- `ApplyGlobalAsync` with `confirmDestructive=false`: DNS apply skips destructive changes entirely (line 262-264), rather than applying safe ones first
- Same pattern for other subsystems

## Expected Outcome

Safe changes should be applied automatically. Only destructive/high-risk changes should require explicit confirmation. The response should indicate which changes were applied and which are pending confirmation.

## Fix Guidance

1. Refactor Apply to apply all non-destructive changes immediately regardless of `confirmDestructive` flag.
2. Only hold back changes marked as destructive/high-risk.
3. Return a response with applied changes and pending destructive changes.

## Acceptance Criteria

- [ ] Safe DNS record creates/updates apply without confirmation
- [ ] Only destructive changes (deletions, prunes) require confirmation
- [ ] Apply response clearly lists what was applied vs what is pending
