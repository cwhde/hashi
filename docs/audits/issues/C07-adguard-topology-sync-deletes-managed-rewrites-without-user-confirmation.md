# C07 - AdGuard topology sync deletes managed rewrites without user confirmation

Priority: Critical

Spec conflicts: non-negotiable rule 7 requires AdGuard changes to go through sync plan preview, apply, result, and audit. Section 16 requires Hashi to never delete user-created rewrites, use Hashi ownership state, avoid touching unknown entries, and sync Hashi rewrites without touching manual rewrites.

## Problem

Manual rewrites created through Hashi are stored with `ManagedByHashi = true`, the same flag used for topology-generated rewrites. When topology sync recomputes desired resource rewrites, it deletes every managed rewrite that is not in the resource topology desired set. That means a Hashi-created manual rewrite can be removed from local desired state simply because it is not a resource rewrite.

The automatic sync path then applies the resulting delete plan with `ConfirmDestructive: true` internally. Passive reconciliation can therefore delete remote AdGuard rewrites without a user previewing and confirming the destructive change.

## Evidence

- `docs/implementation-spec/hashi-v2-implementation-spec.md:24-30` defines ownership and sync-plan safety rules.
- `docs/implementation-spec/hashi-v2-implementation-spec.md:876-895` defines AdGuard rewrite ownership and safe sync behavior.
- `docs/implementation-spec/hashi-v2-implementation-spec.md:1837-1846` says Hashi rewrite sync must not touch manual rewrites.
- `src/Hashi.Infrastructure/Platform/AdGuardSyncService.cs:98-126` creates or updates user-requested rewrites with `ManagedByHashi = true`.
- `src/Hashi.Infrastructure/Platform/AdGuardSyncService.cs:132-135` calls apply with `ConfirmDestructive: true`.
- `src/Hashi.Infrastructure/Platform/AdGuardSyncService.cs:255-325` rebuilds resource-topology desired rewrites and removes every `ManagedByHashi` rewrite whose domain is not in the topology set.
- `src/Hashi.Infrastructure/Sync/SyncOrchestratorService.cs:320-325` runs `SyncManagedRewritesAsync` during global apply.
- `src/Hashi.Infrastructure/Sync/SyncOrchestratorService.cs:421-425` runs the same destructive-capable sync during reconcile.
- `src/Hashi.Api/Features/Platform/PlatformEndpoints.cs:850-853` exposes a one-shot `/sync` endpoint that also bypasses caller-supplied destructive confirmation.

## Expected outcome

Hashi-created manual rewrites and topology-generated rewrites should have distinct ownership/workflow state. Background reconcile should never internally confirm destructive AdGuard deletes.

## Fix guidance

Separate rewrite ownership into at least manual desired rewrites and topology-generated rewrites. Limit topology cleanup to rewrites it generated. Remove the internal `ConfirmDestructive: true` shortcut from background and one-shot sync paths; queue destructive changes for explicit user confirmation through the normal plan/apply UI.

## Acceptance criteria

- A manual Hashi rewrite survives resource topology sync when it is not in the resource desired set.
- Background reconcile cannot delete remote AdGuard rewrites without a user-confirmed apply.
- Topology-generated stale rewrites still appear in a plan as deletes and require confirmation before remote deletion.
- Tests cover manual rewrite preservation, topology stale cleanup, and passive reconcile with destructive changes pending.
