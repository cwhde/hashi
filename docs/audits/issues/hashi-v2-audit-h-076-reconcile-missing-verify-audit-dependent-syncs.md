# H-076: Reconcile Missing Verify, Audit, and Dependent Syncs

**Priority:** High
**Conflict Type:** missing_implementation
**Spec Reference:** Main Spec §25

## Description

`ReconcileAsync()` re-plans and applies but: (1) does not verify the applied state matches desired state post-apply, (2) does not explicitly update hash records, (3) does not write audit entries (no `audit.WriteAsync` call in Reconcile), and (4) does not queue dependent syncs. These are all explicit requirements in the spec's Reconcile phase.

## Evidence

- `ReconcileAsync` re-plans and applies but contains no post-apply verification step
- No explicit hash update
- No `audit.WriteAsync` call
- No dependent sync queuing

## Expected Outcome

Reconcile verifies state, records hashes, writes audit log, and triggers downstream syncs if needed.

## Fix Guidance

1. Add post-apply verification (compare rendered vs applied content).
2. Add explicit hash updates to subsystem entities.
3. Add audit event writes for each reconcile step.
4. Add a mechanism to queue dependent syncs (e.g., DNS sync after firewall apply if ports changed).

## Acceptance Criteria

- [ ] Reconcile audit entries appear in activity log
- [ ] Last applied hashes are updated after successful reconcile
- [ ] Dependent subsystems are synced after upstream changes
- [ ] Failed verification produces a clear error with diff
