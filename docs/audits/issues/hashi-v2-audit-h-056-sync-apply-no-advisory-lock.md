# H-056: SyncOrchestratorService Apply Has No Advisory Lock — Concurrent Applies Can Race

**Priority:** Critical
**Conflict Type:** missing_implementation
**Spec Reference:** Main Spec §25 (Apply must acquire advisory lock per provider/connection)

## Description

`SyncOrchestratorService.ApplyGlobalAsync()` has no lock mechanism. Concurrent Apply calls could race, leading to corrupted state. No SemaphoreSlim, database advisory lock, or similar pattern exists anywhere in the codebase. The spec explicitly requires acquiring an advisory lock per provider/connection before applying changes.

## Evidence

SyncOrchestratorService.ApplyGlobalAsync() runs without any concurrency guard; two simultaneous apply requests can interleave writes.

## Expected Outcome

Only one Apply can run at a time per provider/connection; concurrent requests are rejected or queued with a clear error.

## Fix Guidance

Add a SemaphoreSlim-based or PostgreSQL advisory lock in SyncOrchestratorService. Reject Apply if another is already running for the same scope.

## Acceptance Criteria

- [ ] Two concurrent Apply requests result in one being rejected with a clear error
- [ ] Lock is scoped per provider/connection as spec requires
