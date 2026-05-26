# CI secrets and environment variables

Hashi workflows live in `.gitea/workflows/`. Gitea Actions secrets are configured in the repository under **Settings → Actions → Secrets**.

## Why workflows did not run on feature branches

Until the trigger fix, `ci.yml` and `security.yml` only ran on **push to `main`** and **pull requests targeting `main`**. Pushes to `feat/v2-foundation` (and other branches) did not start CI. `docker-build.yml` had the same branch restriction.

After the fix:

| Workflow        | Push                         | Pull request      | Manual              |
|-----------------|------------------------------|-------------------|---------------------|
| `ci.yml`        | All branches                 | Targeting `main`  | `workflow_dispatch` |
| `security.yml`  | All branches                 | Targeting `main`  | `workflow_dispatch` |
| `docker-build.yml` | `main`, `feat/v2-foundation` (path filters) | — | `workflow_dispatch` |

To run CI without pushing to a configured branch, open a PR into `main` or use **Actions → Run workflow** (`workflow_dispatch`).

## Repository secrets (required)

| Secret               | Used in              | Purpose |
|----------------------|----------------------|---------|
| `REGISTRY_USERNAME`  | `docker-build.yml`   | Login to `git.juzo.io` container registry |
| `REGISTRY_PASSWORD`  | `docker-build.yml`   | Registry password or token for `git.juzo.io` |

`docker-build` fails at the login step if either registry secret is missing.

## Secrets not required

| Name            | Notes |
|-----------------|-------|
| `GITHUB_TOKEN`  | Not used. Gitleaks runs via the CLI binary instead of `gitleaks/gitleaks-action` (GitHub-oriented). |
| Other secrets   | No other `${{ secrets.* }}` references in workflows. |

## Workflow job environment variables

These are set in workflow YAML, not in Gitea Secrets:

| Variable / env | Workflow(s) | Purpose |
|----------------|-------------|---------|
| `DOTNET_SKIP_FIRST_TIME_EXPERIENCE`, `DOTNET_NOLOGO` | `ci.yml` | Faster .NET CI startup |
| `REGISTRY` (`git.juzo.io`) | `docker-build.yml` | Image registry host |
| `IMAGE_NAME` (`juzo/hashi`) | `docker-build.yml` | Image path under registry |
| Postgres service env (`POSTGRES_*`) | `ci.yml` backend job | Ephemeral CI database (not secrets) |
| `ASPNETCORE_ENVIRONMENT`, `DOTNET_ENVIRONMENT`, `HASHI_SKIP_STARTUP_HOOKS` | OpenAPI export script in CI | Set by `scripts/export-openapi.sh` during contract verify |

## OpenAPI verify (CI)

`ci.yml` job `openapi-verify` re-exports `openapi/hashi.json` and regenerates `web/src/lib/api/schema.d.ts`, then fails if they differ from the commit (API contract §30 / `docs/operations/api-contract.md`).

## Path filters (`docker-build.yml`)

Builds run on push when changed paths match `src/**`, `web/**`, `deploy/**`, `openapi/**`, build props, solution file, or the workflow file itself. `hashi.old/**` is ignored. Documentation-only changes do not trigger image builds.
