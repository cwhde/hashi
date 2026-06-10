# H-083: Pulse AllowedScopesJson Never Enforced During Heartbeat Acceptance

**Priority:** Medium
**Conflict Type:** wrong_implementation
**Spec Reference:** §17.2 (Token can only submit heartbeat data for its own agent; Token cannot read config, list resources, or trigger syncs)

**Status:** Not Started
**Branch:** 

## Description

The `AllowedScopesJson` field exists on `PulseAgentEntity` but is never enforced during heartbeat acceptance. The architecture prevents scope violation (only the heartbeat endpoint validates agent tokens), but the spec requires explicit scope enforcement.

## Evidence

- `AllowedScopesJson` field is present on `PulseAgentEntity` but not checked during heartbeat processing
- No scope validation logic exists in the heartbeat acceptance path

## Expected Outcome

Heartbeat acceptance should validate `AllowedScopesJson`. Tokens with empty or invalid scopes should be rejected. Scope enforcement should be documented and tested.

## Fix Guidance

1. Validate `AllowedScopesJson` during heartbeat acceptance.
2. Reject tokens with empty or invalid scopes.
3. Add scope checks to any future Pulse-related endpoints.

## Acceptance Criteria

- [ ] Heartbeat acceptance checks AllowedScopesJson
- [ ] Tokens with empty or invalid scopes are rejected
- [ ] Scope enforcement is documented and tested
