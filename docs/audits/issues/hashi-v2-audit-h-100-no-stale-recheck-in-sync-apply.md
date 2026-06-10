# H-100: No Stale Plan Recheck in Sync Apply

**Priority:** Medium
**Conflict Type:** missing_implementation
**Spec Reference:** §25 (Apply must recheck current state if plan is stale)

**Status:** Not Started
**Branch:** 

## Description

`ApplyGlobalAsync` re-plans from scratch inside Apply, but doesn't compare against the plan the user approved. There is no plan ID verification or content hash comparison. If desired state changed between Plan and Apply, the Apply proceeds with the new state without the user seeing the updated preview.

## Evidence

- `ApplyGlobalAsync` re-plans internally without comparing to the approved plan
- No plan ID or content hash is stored from the Plan phase
- No stale detection logic exists

## Expected Outcome

Apply should verify the plan hasn't gone stale between Plan and Apply. Stale plans should be rejected with a clear error message. Users must re-plan after a stale rejection.

## Fix Guidance

1. Store plan content hash or plan ID from the Plan phase.
2. During Apply, verify the hash/ID matches.
3. If it doesn't match, reject with "stale plan" error and require re-planning.

## Acceptance Criteria

- [ ] Apply verifies plan hasn't gone stale between Plan and Apply
- [ ] Stale plans are rejected with clear error message
- [ ] User must re-plan after stale rejection
