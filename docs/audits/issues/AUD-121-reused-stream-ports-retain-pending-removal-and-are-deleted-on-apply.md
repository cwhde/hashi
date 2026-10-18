# AUD-121: Reused stream ports retain pending removal and are deleted on apply

Priority: P2

Category: Reliability / Traefik entry point lifecycle

Status: open; confirmed by manual source trace.

## Finding

RemoveIfUnusedAsync marks a confirmed stream entry PendingRemoval. If a resource subsequently reuses that same protocol/port before apply, SyncForResourceAsync finds the row but only updates its ResourceId/Label; it never clears PendingRemoval. Global sync plans/removes all pending-removal entries and eventually deletes the reused row, causing the listener state to disagree with the active resource.

## Evidence

src/Hashi.Infrastructure/Platform/TraefikEntryPointService.cs:12-37,39-68; Sync/SyncOrchestratorService.cs:187,466-470.

## Expected outcome and acceptance criteria

Cancel pending removal when an enabled resource reuses the entry, and recheck active references under the apply lock before deleting it. A remove-then-recreate sequence before synchronization must retain the listener without an unintended destructive change.
