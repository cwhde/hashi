# Hashi V2 Addendum Acceptance Evidence

Date: 2026-06-05

## Scope

This records the final TASK-10 acceptance pass after implementing TASK-01 through TASK-09.

Implementation task status:

- TASK-01 data model and contract foundation: implemented.
- TASK-02 security decision engine: implemented.
- TASK-03 subject search and manual actions UI: implemented.
- TASK-04 Cap CAPTCHA challenge flow: implemented.
- TASK-05 blocklist source management: implemented.
- TASK-06 agent-bound connections: implemented.
- TASK-07 internal agent DNS: implemented.
- TASK-08 setup, settings, and background jobs: implemented.
- TASK-09 CI/CD optimization: implemented.
- TASK-10 cross-feature acceptance and hardening: this evidence pass.

## Commands Run

Backend:

- `dotnet format Hashi.slnx --verify-no-changes`: passed.
- `dotnet test Hashi.slnx /p:SkipFrontendBuild=true`: passed, 393 unit tests and 28 integration tests.

Frontend:

- `corepack pnpm install --frozen-lockfile`: passed.
- `corepack pnpm run check`: passed with 0 Svelte diagnostics.
- `corepack pnpm run lint`: passed.
- `corepack pnpm run test`: passed, 12 files and 28 tests.
- `corepack pnpm run build`: passed. Vite reported the existing `uplot` unused-default import warning in `MonitorLatencyChart.svelte`.

Contracts:

- `bash scripts/export-openapi.sh`: passed.
- `bash scripts/generate-api-client.sh`: passed.
- `git diff --exit-code openapi/hashi.json web/src/lib/api/schema.d.ts`: passed.
- `git diff --check`: passed.

CI/CD:

- `bash scripts/validate-ci-optimization.sh`: passed.

Pulse and Docker:

- `make vet`, `make test`, and `make build` could not run locally because `make` is not installed on this host.
- Direct equivalents could not run locally because `go` is not installed on this host.
- Docker smoke builds could not run locally because `docker` is not installed on this host.

## Remaining Local Environment Limits

Pulse Go checks and Docker smoke builds should be verified by the Gitea workflows or a host with Go, Make, and Docker installed.
