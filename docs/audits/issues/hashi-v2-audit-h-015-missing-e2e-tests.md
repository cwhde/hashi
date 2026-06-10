# H-015: Missing E2E Tests for Core User Flows

**Priority:** High
**Conflict Type:** missing_implementation
**Spec Reference:** Main Spec §31 (Test Strategy - E2E tests)

**Status:** Fixed
**Branch:** h/tests

## Description

The implementation spec requires comprehensive E2E tests:
```
E2E tests:
- First setup flow.
- Passkey registration with browser-supported test harness.
- Resource creation.
- DNS import.
- Middleware editor validation.
- Status page public view.
- App dashboard public view.
- Custom script save/manual run flow with fake host.
```

The actual E2E test suite (`web/e2e/setup.spec.ts`) contains only 4 tests:
1. Setup wizard navigation
2. Login page elements
3. Public status page
4. Optional step skip

Missing E2E tests:
- ❌ Passkey registration flow
- ❌ Resource creation flow
- ❌ DNS import flow
- ❌ Middleware editor validation
- ❌ App dashboard public view
- ❌ Custom script save/manual run flow
- ❌ Traefik config render/apply
- ❌ Firewall host configuration
- ❌ Security dashboard interaction
- ❌ Blocklist management

## Evidence

```typescript
// web/e2e/setup.spec.ts - only 4 tests
test.describe('Setup Wizard', () => {
  test('shows setup steps', async ({ page }) => { ... });
  test('shows login page after setup', async ({ page }) => { ... });
  test('shows public status page', async ({ page }) => { ... });
  test('allows skipping optional step', async ({ page }) => { ... });
});
```

The spec requires at least 8 specific E2E test scenarios. Only 4 are implemented, and they only cover the setup wizard and basic page visibility.

## Expected Outcome

- E2E tests cover all major user flows
- Passkey registration is tested with browser harness
- Resource CRUD is tested end-to-end
- DNS import is tested
- Public pages are tested

## Fix Guidance

Add E2E tests for:
1. Passkey registration (using WebAuthn test harness)
2. Resource creation/edit/delete
3. DNS record import
4. Middleware editor YAML validation
5. Public dashboard and status page views
6. Script creation and manual execution
7. Security subject search and manual block/allow

## Acceptance Criteria

- [ ] Passkey registration E2E test exists
- [ ] Resource creation E2E test exists
- [ ] DNS import E2E test exists
- [ ] Middleware editor validation E2E test exists
- [ ] Public dashboard E2E test exists
- [ ] Public status page E2E test exists
- [ ] Script save/manual run E2E test exists
