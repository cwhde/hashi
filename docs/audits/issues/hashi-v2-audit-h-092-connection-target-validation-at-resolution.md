# H-092: Connection Target Validation at Resolution Time, Not Save Time

**Priority:** Medium
**Conflict Type:** wrong_implementation
**Spec Reference:** Addendum §9.5 (Before saving an agent-bound connection: agent must exist, agent must have heartbeat unless explicitly allowed, port must be valid)

**Status:** Fixed
**Branch:** h/backend-quality

## Description

Connection target validation happens at resolution time, not at save time. Invalid combinations (e.g., PulseAgent mode without `PulseAgentId`) are saved to the database and only fail when resolution is attempted.

## Evidence

- Invalid connection targets can be persisted to the database
- Failures only surface during resolution, not during save

## Expected Outcome

Saving a connection target with invalid mode/agent/port should fail with a validation error. Invalid targets should never be persisted to the database. Validation errors should clearly explain what is wrong.

## Fix Guidance

1. Add validation at save time that checks: agent exists for pulse_agent mode, port is valid for all modes, IP mode is valid.
2. Reject invalid targets before persisting.
3. Return clear validation error messages.

## Acceptance Criteria

- [ ] Saving a connection target with invalid mode/agent/port fails with validation error
- [ ] Invalid targets are never persisted to the database
- [ ] Validation errors clearly explain what is wrong
