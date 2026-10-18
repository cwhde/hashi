# AUD-092: Cancelled reauthentication can still display successful mutations

Priority: P2

Category: UI reliability / operation outcomes

Status: open; confirmed by manual source tracing.

## Finding

AdGuard sync/delete and script delete await withReauth then display a success message without checking its null cancellation/failure result. AdGuard sync additionally ignores succeeded=false responses. Generic connection and DNS connection validation similarly ignore semantic failure and report validated. This can cause operators to believe a failed safety-critical change or validation actually completed.

## Evidence

web/src/routes/(admin)/adguard/+page.svelte:100-113,215-227,248-257; web/src/routes/(admin)/scripts/+page.svelte:155-163; web/src/routes/(admin)/connections/+page.svelte:90-95; web/src/routes/(admin)/dns/+page.svelte:100-107,130-137

Related AUD083 covers setup progression consequences.

## Expected outcome

Distinguish cancellation from success, inspect operation success fields, and preserve actual API errors. Extend setup-specific AUD083 handling to admin pages.

## Acceptance criteria

Cancelled/failed reauthentication never displays deletion/sync success; HTTP200 failed operation and SSH validation results show failure and remain retryable.
