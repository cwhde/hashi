# Hardening Checklist

## Runtime

- Run Hashi behind Traefik with TLS termination.
- Restrict admin API (`8080`) to management networks.
- Public ports `8081` (status) and `8082` (apps) expose only enabled dashboard/status data.
- Store SSH, DNS, OIDC, and notification secrets in the vault.

## CI

- `dotnet test` unit suite on every push.
- Integration tests (PostgreSQL + SSH testcontainers) on CI runners with Docker.
- OpenAPI export must succeed without database (`HASHI_SKIP_STARTUP_HOOKS=1`).
- Security workflow scans dependencies and container images.

## Release

- Threat model reviewed for vault, SSH, OIDC, and edge forward-auth paths.
- Document accepted risks for any failing security scan.
- Verify fresh Docker Compose bootstrap end-to-end before tagging a release.
