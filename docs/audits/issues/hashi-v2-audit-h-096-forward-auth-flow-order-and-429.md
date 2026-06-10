# H-096: Forward Auth Flow Order Wrong and Missing 429 Rate-Limit Response

**Priority:** Medium
**Conflict Type:** wrong_implementation
**Spec Reference:** §11 (Forward auth returns 429 for rate-limited traffic); Addendum §14 (11-step evaluation order)

**Status:** In Progress
**Branch:** h/sync-engine
**Branch:** 

## Description

The decision flow has 16 steps instead of the spec's 11. Missing from the expected flow: rate-limiting step (should produce 429 response). The implemented order also intermixes manual allow processing with block checks rather than having clean sequential steps matching the spec's priority order.

## Evidence

- Forward auth decision flow contains 16 steps vs spec's 11
- No rate-limiting step exists in the flow
- 429 response mode is not available
- Manual allow and block checks are intermixed rather than sequential

## Expected Outcome

Each step should execute in the spec-specified order. Rate-limiting should occur at the correct step position (between challenge and rule evaluation). 429 response should be distinct from 403.

## Fix Guidance

1. Reorder to match spec's exact 11-step sequence.
2. Add rate-limit step at the specified position (between challenge and rule evaluation).
3. Ensure 429 response mode is available.

## Acceptance Criteria

- [ ] Each step executes in the spec-specified order
- [ ] Rate-limiting occurs at the correct step
- [ ] 429 response is distinct from 403
