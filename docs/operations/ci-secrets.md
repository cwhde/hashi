# CI secrets and environment variables

Hashi workflows live in `.gitea/workflows/`. Gitea Actions secrets are configured in the repository under **Settings → Actions → Secrets**.

## Workflow triggers (current)

| Workflow | Push | Pull request | Schedule | Manual |
|----------|------|--------------|----------|--------|
| `ci.yml` | `main`, `feat/v2-foundation` | → `main` | — | `workflow_dispatch` |
| `security.yml` | `main` only | → `main` | Weekly Mon 06:00 UTC | `workflow_dispatch` |
| `docker-build.yml` | `main` + tags `v*.*.*` | — | — | `workflow_dispatch` |
| `docker-build-pulse.yml` | `main` + tags `v*.*.*` or `pulse-v*.*.*` | — | — | `workflow_dispatch` |

**Feature branch pushes** run **only the CI jobs whose paths changed** (backend, frontend, openapi-verify, pulse-agent). They do **not** run Security scans or Docker image publish on every commit.

Integration tests use Testcontainers when Docker is available; they skip gracefully when the daemon is missing or misconfigured.

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

## OpenAPI verify

`ci.yml` job `openapi-verify` re-exports `openapi/hashi.json` and regenerates `web/src/lib/api/schema.d.ts`, then fails if they differ from the commit.
