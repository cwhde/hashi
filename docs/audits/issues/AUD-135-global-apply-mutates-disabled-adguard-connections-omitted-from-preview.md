# AUD-135: Global apply mutates disabled AdGuard connections omitted from preview

Priority: P1

Category: Apply safety / disabled connections

Status: open; confirmed by manual source trace.

## Finding

Global planning filters AdGuardConnections by Enabled, while global apply and dependent reconciliation enumerate every AdGuard connection. AdGuardSyncService does not enforce Enabled itself. A disabled connection can therefore receive remote writes/deletions absent from the reviewed global plan. This violates both disabled-state semantics and plan/apply equivalence.

## Evidence

src/Hashi.Infrastructure/Sync/SyncOrchestratorService.cs:248-250 versus583-590,765 onward; Platform/AdGuardSyncService.cs:200-236.

## Expected outcome and acceptance criteria

Use the same enabled connection set in planning/apply/dependencies and validate it again before mutation. A disabled AdGuard endpoint must receive no network mutations and cannot be introduced only at apply time.
