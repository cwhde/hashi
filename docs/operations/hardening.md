# Hardening Checklist

## Runtime

- Run Hashi behind Traefik with TLS termination.
- Restrict admin API (`8080`) to management networks.
- Public ports `8081` (status) and `8082` (apps) expose only enabled dashboard/status data.
- Store SSH, DNS, OIDC, and notification secrets in the vault.

## CI

- `dotnet test` unit suite on every push.
- Integration tests use the CI PostgreSQL service (with readiness retry); Testcontainers is used locally when Docker is available.
- SSH Testcontainers tests are **skipped in CI** by default (`CI=true`); opt in with `HASHI_RUN_SSH_INTEGRATION_TESTS=1` on runners that support them.
- OpenAPI export must succeed without database (`HASHI_SKIP_STARTUP_HOOKS=1`).
- Security workflow scans dependencies and container images.

## Threat model (summary)

| Surface | Risk | Mitigation |
|---------|------|------------|
| Vault / secrets | Credential theft | Passkey + recovery key; secrets encrypted at rest; reauth for sensitive API writes |
| SSH remote exec | Host compromise | Vault-stored credentials; audited script runs; connection-scoped deployment paths |
| Edge forward-auth | Session hijack, auth bypass | HttpOnly cookies; DB-backed sessions; blocklist + adaptive abuse scoring |
| OIDC callback | Open redirect, token replay | State parameter; short-lived pending logins; issuer validation |
| Public ports 8081/8082 | Data leakage | Feature toggles; only dashboard-enabled resources and monitor summaries exposed |

## Release

- Document accepted risks for any failing security scan.
- Verify fresh Docker Compose bootstrap end-to-end before tagging a release.
- Run Playwright setup smoke (`web/e2e/setup.spec.ts`) and integration setup flow tests before release.
