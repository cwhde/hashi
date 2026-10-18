# AUD-034: Subsystem apply endpoints bypass the shared sync lock

Priority: P2 — overlapping applies can race on the same provider or remote files

Category: Concurrency / sync consistency

Spec reference: implementation spec §25 requires advisory locking for apply and stale-plan rechecks; addendum §4 requires provider changes to flow through reviewed plan/apply/result workflows.

## Finding

Global apply and reconcile acquire a process-level `ApplyLock` plus a PostgreSQL advisory lock. The separate DNS, Traefik, and firewall apply endpoints call their provider services directly, and those services do not acquire either shared lock. A direct subsystem apply can therefore overlap a global apply or background reconcile for the same connection/host. The calls can both recheck the same prior state, then interleave remote writes, service reloads, and last-applied state updates; a returned success or stored hash may describe only one operation's view of the resulting remote configuration.

## Evidence

- `src/Hashi.Infrastructure/Sync/SyncOrchestratorService.cs`: `ApplyGlobalAsync` and `ReconcileAsync` acquire `_applyCoordinator.ApplyLock` and `PostgresAdvisoryLock.TryAcquireAsync`.
- `src/Hashi.Api/Features/Dns/DnsEndpoints.cs`: connection sync apply calls `DnsConnectionService.ApplyPlanWithSyncRunAsync` directly.
- `src/Hashi.Api/Features/Traefik/TraefikEndpoints.cs`: connection apply, rollback, and generic apply call `TraefikSyncService` directly.
- `src/Hashi.Api/Features/Firewall/FirewallEndpoints.cs`: apply and rollback call `FirewallApplyService` directly.
- `DnsConnectionService`, `TraefikSyncService`, and `FirewallApplyService` contain no corresponding shared process/advisory lock acquisition.

## Expected outcome

All operations that mutate provider or remote state should share a lock keyed consistently by subsystem and connection/host, including manual and background paths. Recheck or invalidate the approved plan after lock acquisition, not before it.

## Acceptance criteria

- A direct apply cannot run concurrently with a global apply or reconcile that touches the same provider/host.
- Locking works across multiple Hashi processes through PostgreSQL advisory locks, not only an in-memory semaphore.
- Stale-plan validation occurs after obtaining the relevant lock and before remote mutation.
- Rejected concurrent operations return a clear conflict and leave remote state and last-applied hashes unchanged.
- Independent providers may still apply concurrently where no shared dependency exists.
