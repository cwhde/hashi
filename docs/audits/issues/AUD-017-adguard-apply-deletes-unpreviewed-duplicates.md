# AUD-017: AdGuard apply deletes duplicate rewrites outside the reviewed plan

Priority: P1 — destructive apply safety

Category: Bug / unsafe unplanned provider mutation

Spec reference: implementation spec §§7.4 and 24 require provider changes to be planned, previewed, and confirmed when destructive.

## Finding

`ApplyPlanAsync` calls `CleanupDuplicateRemoteRewritesAsync` before applying the changes in the plan. That cleanup groups all remote entries whose domain exists in Hashi's managed desired-state table, keeps one entry, and deletes every other entry. The plan does not include those deletes, and `RequiresConfirmation` is calculated only from planned `delete` changes. As a result, a user can approve a non-destructive or empty plan and still cause unpreviewed provider-side deletions. The AdGuard API does not identify rewrite ownership, so a duplicate may have been created outside Hashi; the cleanup may remove a user's manually managed entry.

## Evidence

- `src/Hashi.Infrastructure/Platform/AdGuardSyncService.cs`: `ApplyPlanAsync` calls `CleanupDuplicateRemoteRewritesAsync` before iterating the plan changes.
- `PlanSyncAsync` builds the response and `RequiresConfirmation` from its `changes` list; it does not represent duplicate cleanup as changes.
- `CleanupDuplicateRemoteRewritesAsync` deletes all but one remote row for each duplicated managed domain, without proving ownership of the deleted rows.
- `ApplySafePlanAsync` does not perform this cleanup, so the two apply paths have different unlisted side effects.

## Expected outcome

Every remote mutation must appear in the plan and be bound to its plan ID. Duplicate cleanup must be separately previewed and require destructive confirmation, or be removed until ownership can be determined safely. Preserve unrelated/manual remote state.

## Acceptance criteria

- No remote delete occurs unless represented by the reviewed plan and covered by the confirmation flow.
- Duplicate entries with ambiguous ownership are reported for manual resolution instead of automatically deleted.
- Stale-plan detection covers the exact duplicate set and answers that would be deleted.
- Safe apply has no unpreviewed provider mutations.
