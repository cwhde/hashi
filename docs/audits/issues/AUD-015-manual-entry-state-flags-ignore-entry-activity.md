# AUD-015: Manual security state flags ignore entry activity

Priority: P3 — security dashboard/state correctness

Spec reference: implementation addendum §§3–5 requires explicit security subject state and manual allow/block lifecycle to remain consistent with active policy entries.

## Finding

Manual entry mutations call `UpsertSubjectStateAsync`, which sets `ManualAllowActive` or `ManualBlockActive` and the subject's current state without checking whether the entry is enabled or already expired. The create path does this even when `Enabled=false`; update does it when disabling an entry as well. `CreateBlockAsync` also unconditionally sets a block state after creating the entry. The decision engine filters on enabled and unexpired entries, so enforcement can disagree with the persisted state and dashboard during that interval; the cleanup worker repairs manual-block flags later, but no equivalent cleanup recomputes manual-allow flags.

## Evidence

- `src/Hashi.Infrastructure/Platform/SecuritySubjectOperationsService.cs`: `CreateManualEntryAsync` and `UpdateManualEntryAsync` call `UpsertSubjectStateAsync` after any create/update; that helper unconditionally sets the corresponding active flag.
- The same file's `CreateBlockAsync` calls `ApplyBlockStateAsync` without conditioning on `entry.Enabled` or expiry.
- `SecurityDecisionService.LoadMatchingManualEntriesAsync` filters entries by enabled status and expiry, so these denormalized state flags can diverge from actual enforcement.
- `SecurityAddendumJobWorker` periodically reconciles manual-block flags but does not rebuild `ManualAllowActive` from active allow entries.

## Expected outcome

Derive state flags from active entries (enabled and unexpired), and recompute them after every create, update, disable, expire, and delete operation. Keep current state and dashboard values consistent with the decision engine.

## Acceptance criteria

- Creating or updating a disabled/expired allow or block does not set its active state flag.
- Disabling or expiring the last active entry clears its flag promptly.
- Deleting one of multiple entries preserves the flag while another active entry remains.
- Manual state mutation uses a shared recomputation path for allow and block entries.
