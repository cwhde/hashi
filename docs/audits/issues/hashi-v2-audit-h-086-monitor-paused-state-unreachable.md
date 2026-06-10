# H-086: Monitor Paused State Unreachable

**Priority:** Low
**Conflict Type:** wrong_implementation
**Spec Reference:** §18.3 (Status states include Paused)

**Status:** Not Started
**Branch:** 

## Description

The Paused monitor state exists in code normalization but is unreachable — there is no mechanism to pause a monitor endpoint. The Enabled toggle disables it entirely. No API endpoint or UI control sets Paused state.

## Evidence

- Paused state is defined in status normalization logic but never set by any code path
- Enabled toggle fully disables the endpoint rather than pausing
- No API endpoint accepts a "pause" action

## Expected Outcome

Monitor endpoints should be pausable via API/UI. Paused endpoints should retain their configuration but stop running checks. They should show "Paused" status and resume when unpaused.

## Fix Guidance

1. Add a Paused status that can be set via API.
2. Paused endpoints retain their configuration but stop running checks.
3. Add UI control for pause/unpause.

## Acceptance Criteria

- [ ] Monitor endpoints can be paused via API
- [ ] Paused endpoints show "Paused" status in UI
- [ ] Paused endpoints retain configuration and resume when unpaused
