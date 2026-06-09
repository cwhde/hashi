# H-082: Service-Sync Vault Unavailable Does Not Pause Gracefully

**Priority:** Medium
**Conflict Type:** wrong_implementation
**Spec Reference:** §8 (If service-sync vault cannot unlock, provider sync jobs pause and surface critical health warning)

## Description

Services needing secrets throw `InvalidOperationException` rather than gracefully pausing and retrying when the service-sync vault is unavailable. No health check endpoint or dashboard indicator exposes this state.

## Evidence

- Provider sync jobs crash with `InvalidOperationException` when vault is locked
- No health check endpoint reports vault-locked state
- Dashboard has no indicator for vault unavailability

## Expected Outcome

Sync jobs should pause gracefully when vault is unavailable and surface a critical health warning. Jobs should automatically resume when the vault is unlocked.

## Fix Guidance

1. Add a paused state to background jobs when vault is unavailable.
2. Surface critical health warning in dashboard and health endpoints.
3. Implement retry/resume logic that activates when vault state changes to unlocked.

## Acceptance Criteria

- [ ] Sync jobs pause gracefully when vault is locked
- [ ] Dashboard shows critical health warning for vault-locked state
- [ ] Jobs automatically resume when vault is unlocked
