# CI secrets and environment variables

Hashi workflows live in `.gitea/workflows/`. Gitea Actions secrets are configured in the repository under **Settings → Actions → Secrets**.

## Workflow triggers (current)

| Workflow | Push | Pull request | Schedule | Manual |
|----------|------|--------------|----------|--------|
| `ci.yml` | `main`, `feat/v2-foundation` | → `main` | — | `workflow_dispatch` |
| `security.yml` | `main` only | → `main` | Weekly Mon 06:00 UTC | `workflow_dispatch` |
| `docker-build.yml` | `main` + tags `v*.*.*` | — | — | `workflow_dispatch` |
| `docker-build-pulse.yml` | `main` + tags `v*.*.*` or `pulse-v*.*.*` | — | — | `workflow_dispatch` |

**Feature branch pushes** run **only the CI jobs whose paths changed** (backend, web, pulse-agent). They do **not** run Security scans or Docker image publish on every commit.

Integration tests use the CI PostgreSQL service when `ConnectionStrings__Hashi` is set (see `ci.yml`). They skip gracefully when the database is unavailable. SSH container tests are skipped in CI unless `HASHI_RUN_SSH_INTEGRATION_TESTS=1`.

**Release images:** push a semver tag (`v1.0.0`) or merge to `main`, or use **Actions → Run workflow**. Main app image: `git.juzo.io/juzo/hashi`. Pulse agent: `git.juzo.io/juzo/hashi-pulse`.

## Repository secrets (required for Docker publish)

| Secret | Used in | Purpose |
|--------|---------|---------|
| `REGISTRY_USERNAME` | `docker-build.yml`, `docker-build-pulse.yml` | Login to `git.juzo.io` container registry |
| `REGISTRY_PASSWORD` | `docker-build.yml`, `docker-build-pulse.yml` | Registry password or token |

`docker-build` fails at login if either secret is missing.

## Secrets not required

| Name | Notes |
|------|-------|
| `GITHUB_TOKEN` | Not used. Gitleaks runs via CLI binary. |

## CI failure reference (runs 40–42)

| Run | Workflow | Typical failure cause |
|-----|----------|----------------------|
| 40 | `ci.yml` | Backend build ran frontend `pnpm` without `SkipFrontendBuild`; frontend `pnpm install --frozen-lockfile`; OpenAPI drift |
| 41 | `docker-build.yml` | Missing registry secrets or unnecessary run on feature branch |
| 42 | `security.yml` | Moderate transitive JWT advisories failing `dotnet list package --vulnerable` |

Fixes applied: `SkipFrontendBuild` in CI backend job; eslint/navigation fixes; security job only fails on High/Critical; docker-build limited to `main` + version tags.

## Gitea runner cache timeouts

If CI logs show:

```text
Warning: Failed to restore: getCacheEntry failed: connect ETIMEDOUT 172.26.0.2:35735
pnpm cache is not found
```

That is a **runner/infrastructure** issue: the act_runner cannot reach the Gitea Actions cache API (often the cache service address in `config.yaml` is wrong or firewalled). CI no longer uses `actions/setup-node` `cache: pnpm` to avoid waiting on a broken cache server.

**Fix on the server (pick one):**

1. In act_runner `config.yaml`, set a reachable cache host or disable cache (`cache.enabled: false`).
2. Ensure the runner container can reach the Gitea instance cache port (not only `172.26.0.2` from an old Docker network).
3. Upgrade act_runner if using an older build with cache bugs.

Backend `pnpm` exit code **127** means Node/pnpm was not on PATH when MSBuild tried to build the SPA; `Directory.Build.props` sets `SkipFrontendBuild=true` when `CI=true` so `dotnet test` no longer invokes `pnpm`.

## OpenAPI verify

`ci.yml` job `web` runs frontend checks and OpenAPI verification in one job (single `pnpm install`). It re-exports `openapi/hashi.json`, regenerates `web/src/lib/api/schema.d.ts`, then fails if they differ from the commit.
