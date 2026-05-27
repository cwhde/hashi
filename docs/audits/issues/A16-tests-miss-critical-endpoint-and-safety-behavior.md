# A16 - Tests miss critical endpoint and safety behavior

Priority: High

Spec conflicts: test requirements in section 31 plus non-negotiable rules 1, 2, 7, 17, 19, 20, 21, and 23.

## Problem

The current tests cover some useful units, but several critical spec guarantees are untested or are tested at the wrong layer. This allowed the DNS endpoint plan/apply bug to exist even though DNS plan/apply tests pass at the service layer. Edge SSO tests seed an OIDC provider for SSO-required paths, so they do not catch fail-open behavior when no provider exists. Public page e2e only checks that pages return less than 500 and does not verify public-port mode or data loading.

Integration tests can silently skip major coverage when Docker/Postgres/SSH containers are unavailable. SSH integration tests are explicitly skipped in CI unless `HASHI_RUN_SSH_INTEGRATION_TESTS=1`, despite the spec requiring SSH/SFTP test-container validation.

## Evidence

- `tests/Hashi.IntegrationTests/HetznerDnsPlanApplyTests.cs:93-99` tests service plan/apply directly, not the HTTP plan/apply contract.
- `tests/Hashi.UnitTests/EdgeAuthServiceTests.cs:63-75` tests SSO required only after seeding a provider.
- `tests/Hashi.UnitTests/EdgeAuthServiceTests.cs:137-147` tests strict mode only after seeding a provider.
- `web/e2e/setup.spec.ts:24-27` only checks that `/status-page` loads with status less than 500.
- `tests/Hashi.IntegrationTests/IntegrationTestPostgres.cs:102-105` returns without integration DB coverage if Docker is unavailable.
- `tests/Hashi.IntegrationTests/SshRemoteExecutorTests.cs:143-148` skips SSH tests in CI unless an opt-in environment variable is set.
- `tests/Hashi.UnitTests/DnsSafetyTests.cs:1-56` does not cover unowned matching-record update conflicts.

## Expected outcome

Tests should exercise API contracts and high-risk safety invariants, not only internal service happy paths. CI should fail when required integration dependencies are missing, or clearly separate optional local-only tests from required CI coverage.

## Fix guidance

Add endpoint-level DNS plan/apply tests, unowned DNS collision tests, fail-closed SSO tests without providers, public-port data-fetch tests, Traefik YAML parse tests, firewall idempotency tests, AdGuard plan tests, and vault service-sync boundary tests. Make CI-required integration tests fail when prerequisites are missing instead of silently returning.

## Acceptance criteria

- DNS HTTP plan/apply test fails before A03 is fixed and passes after.
- Edge SSO no-provider fail-closed tests exist.
- Public dashboard/status e2e verifies 8081/8082 mode without admin API access.
- SSH and PostgreSQL integration coverage is either required in CI or explicitly split into a separate required runner.
