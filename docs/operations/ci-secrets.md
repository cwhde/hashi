# CI secrets and environment variables

Hashi workflows live in `.gitea/workflows/`. Gitea Actions secrets are configured in the repository under **Settings -> Actions -> Secrets**.

## Workflow triggers

| Workflow | Push | Pull request | Schedule | Manual |
|----------|------|--------------|----------|--------|
| `ci.yml` | all branches, path-filtered | all branches, path-filtered | none | `workflow_dispatch` |
| `security.yml` | all branches, heavy scans path-filtered | all branches | Weekly Mon 06:00 UTC | `workflow_dispatch` |
| `docker-build.yml` | `main` + tags `v*.*.*` | none | none | `workflow_dispatch` |
| `docker-build-pulse.yml` | `main` + tags `v*.*.*` or `pulse-v*.*.*` | none | none | `workflow_dispatch` |
| `docker-build-old.yml` | `main` when `hashi.old/**` changes | none | none | `workflow_dispatch` |

Feature branch pushes run only the CI jobs whose paths changed: backend, web, pulse-agent, or CI optimization validation. Security uses a cheap path filter so docs-only pushes skip the heavy scanner job, while scheduled/manual runs still execute all security coverage. Docker image publish runs only on `main`, release tags, or manual dispatch.

Integration tests start PostgreSQL via Testcontainers when `/var/run/docker.sock` is available on the runner. They skip gracefully when Docker is unavailable. SSH container tests are skipped in CI unless `HASHI_RUN_SSH_INTEGRATION_TESTS=1`.

Release images: push a semver tag (`v1.0.0`) or merge to `main`, or use **Actions -> Run workflow**. Main app image: `git.juzo.io/juzo/hashi`. Pulse agent: `git.juzo.io/juzo/hashi-pulse`.

## Repository secrets

| Secret | Used in | Purpose |
|--------|---------|---------|
| `REGISTRY_USERNAME` | `docker-build.yml`, `docker-build-pulse.yml`, `docker-build-old.yml`, `security.yml` image-digest scan | Login to `git.juzo.io` container registry |
| `REGISTRY_PASSWORD` | `docker-build.yml`, `docker-build-pulse.yml`, `docker-build-old.yml`, `security.yml` image-digest scan | Registry password or token |

Docker publish workflows fail at login if either publish secret is missing. `security.yml` attempts to scan the exact pushed main/tag image digest when the registry image is already available; if registry login or image resolution is unavailable, it falls back to building a local scan image.

## Secrets not required

| Name | Notes |
|------|-------|
| `GITHUB_TOKEN` | Not needed for normal CI/security. Pulse release attachment uses the Gitea-provided token on tag builds. |

## CI failure reference

| Workflow | Typical failure cause |
|----------|----------------------|
| `ci.yml` | Backend format/test/build failure, frontend check/lint/test/build failure, OpenAPI drift, Pulse vet/test/build failure |
| `security.yml` | Secret finding, High/Critical vulnerable package, Semgrep error finding, Trivy High/Critical finding |
| Docker publish workflows | Missing registry secrets, Dockerfile build failure, registry push failure |

## Gitea runner cache timeouts

If CI logs show:

```text
Warning: Failed to restore: getCacheEntry failed: connect ETIMEDOUT 172.26.0.2:35735
pnpm cache is not found
```

That is a runner/infrastructure issue: the act_runner cannot reach the Gitea Actions cache API, often because the cache service address in `config.yaml` is wrong or firewalled. Workflows use explicit `actions/cache` restore/save steps with `continue-on-error: true` for NuGet, pnpm, Go, Playwright, ShellCheck, Semgrep, Gitleaks, and Trivy caches, so cache outages should warn rather than fail required checks.

Fix on the server:

1. In act_runner `config.yaml`, set a reachable cache host or disable cache (`cache.enabled: false`).
2. Ensure the runner container can reach the Gitea instance cache port, not only an old Docker-network address.
3. Upgrade act_runner if using an older build with cache bugs.

Backend `pnpm` exit code 127 means Node/pnpm was not on PATH when MSBuild tried to build the SPA. `Directory.Build.props` sets `SkipFrontendBuild=true` when `CI=true` so `dotnet test` no longer invokes `pnpm`.

## Docker build optimization

Main, Pulse, and legacy image workflows keep `linux/amd64,linux/arm64` and registry BuildKit caches. The main Dockerfile pins Node and .NET SDK build stages to `$BUILDPLATFORM`, maps Docker `TARGETARCH` to .NET Linux runtime identifiers, and uses BuildKit cache mounts for pnpm and NuGet. The Pulse Dockerfile pins the Go build stage to `$BUILDPLATFORM`, uses `TARGETOS`/`TARGETARCH`, and caches Go module/build data. The legacy Dockerfile keeps its target-platform dependency install behavior for native modules and adds an npm cache mount.

`scripts/validate-ci-optimization.sh` checks these invariants and is run by `ci.yml` when workflow or Docker build files change.

## OpenAPI verify

`ci.yml` job `web` runs frontend checks and OpenAPI verification in one job (single `pnpm install`). It re-exports `openapi/hashi.json`, regenerates `web/src/lib/api/schema.d.ts`, then fails if they differ from the commit.
