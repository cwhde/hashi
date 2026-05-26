# A03 - DNS sync plan/apply endpoint flow is unusable

Priority: High

Spec conflicts: non-negotiable rules 7, 20, and 22; DNS plan/apply/reconcile requirements in section 24.

## Problem

The DNS API returns a random plan id from `/sync/plan`, but `/sync/apply` recomputes a new plan and compares the new random id with the request id. In normal use, the ids will not match, so apply returns `Plan is stale. Re-run sync/plan.`

The service-level tests call `DnsConnectionService.PlanSyncAsync` and `ApplyPlanAsync` directly, so they do not catch the endpoint bug. The apply path also does not persist created provider record ids or update local desired/applied hashes, making later reconciliation less reliable.

## Evidence

- `src/Hashi.Api/Features/Dns/DnsEndpoints.cs:132-160` maps plan/apply and recomputes the plan during apply.
- `src/Hashi.Infrastructure/Dns/DnsConnectionService.cs:245-255` creates every `DnsSyncPlan` with `Guid.NewGuid()`.
- `src/Hashi.Infrastructure/Dns/DnsConnectionService.cs:257-302` applies plan changes but does not persist create results back to `dns_records`.
- `tests/Hashi.IntegrationTests/HetznerDnsPlanApplyTests.cs:93-99` tests service methods directly instead of the HTTP plan/apply contract.

## Expected outcome

The plan endpoint should persist or deterministically identify the exact plan preview. The apply endpoint should apply that same plan after confirming it is still valid, then record a sync run, diff summary, results, provider ids, and audit entry.

## Fix guidance

Persist DNS plans/diffs under the shared sync model or add a DNS plan table keyed by `PlanId`. On apply, load the stored plan, revalidate freshness against the current provider state hash, then apply. After creates, store returned provider record ids and applied hashes. Add endpoint-level integration tests for plan then apply.

## Acceptance criteria

- Calling `/api/dns/connections/{id}/sync/plan` followed by `/sync/apply` with the returned `planId` succeeds when provider state is unchanged.
- Stale plans are rejected based on provider/desired-state revision, not a freshly generated random id.
- Created DNS records persist provider ids.
- A sync run/diff/audit record is created for apply.
