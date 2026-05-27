# A08 - AdGuard writes bypass plan/preview/apply/audit flow

Priority: High

Spec conflicts: non-negotiable rules 7 and 20; AdGuard section 15.5 and global sync section 24.

## Problem

AdGuard rewrite creation, deletion, and sync write directly to the remote AdGuard API. There is no plan preview, no apply confirmation, no result object, and no audit entry. Remote delete is also best-effort: failures are swallowed and the local row is removed anyway, which can leave provider state diverged without a clear failure result.

The spec requires DNS, Traefik, firewall, and AdGuard changes to go through sync plan preview/apply/result/audit. It also requires deterministic domain/answer matching plus Hashi ownership state.

## Evidence

- `src/Hashi.Infrastructure/Platform/AdGuardSyncService.cs:103-129` saves a rewrite and immediately calls `PushToAdGuardAsync`.
- `src/Hashi.Infrastructure/Platform/AdGuardSyncService.cs:59-91` attempts remote delete, swallows all errors, then removes the local row.
- `src/Hashi.Infrastructure/Platform/AdGuardSyncService.cs:132-168` syncs managed rewrites directly.
- `src/Hashi.Api/Features/Platform/PlatformEndpoints.cs:699-739` exposes direct rewrite upsert/delete/sync endpoints.

## Expected outcome

AdGuard changes should be planned from desired state and current remote rewrites, previewed, applied only after validation/confirmation, recorded as a sync run with per-change result, and audited. Remote failures should not silently erase local desired/ownership state.

## Fix guidance

Introduce an AdGuard adapter with `Plan`, `Apply`, and `Reconcile`. Treat manual rewrite edits as desired-state changes that queue a sync plan. Preserve local desired rows on remote errors and surface drift. Add audit events for create/update/delete/apply.

## Acceptance criteria

- Manual rewrite changes return a plan or queue a plan instead of writing remote state immediately.
- Remote errors are visible and do not delete local desired state.
- Apply records sync run, diff, result, and audit event.
- Tests cover remote failure preserving local desired state.
