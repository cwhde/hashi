# A15 - CI is red and workflow coverage does not satisfy the spec

Priority: High

Spec conflicts: non-negotiable rule 23 and CI/CD section 30.

## Problem

The latest commit does not pass CI. Backend formatting fails and generated OpenAPI/frontend API artifacts are stale. The Docker build workflow was canceled on the latest commit, so there is no successful container build for the audited head.

Workflow coverage also misses some spec requirements. The security workflow runs a Trivy filesystem scan but not a built container image scan. The CI changes filter runs the web/OpenAPI job only for `web/`, `openapi/`, `scripts/`, or CI-file changes on push, which means backend-only API changes can skip OpenAPI verification on push. The repository suppresses medium/high .NET vulnerability warnings globally, which weakens the "dependency audits" expectation.

## Evidence

- Gitea Actions: commit `af8c10833095f7a922731f83fa8a228ac95ab64d`, `ci.yml` run 180 failed, `docker-build.yml` run 181 canceled, `security.yml` run 182 succeeded.
- CI backend job 503 failed `dotnet format` with whitespace errors in `src/Hashi.Infrastructure/Platform/MonitorCheckWorker.cs` lines 151-233.
- CI web job 504 failed OpenAPI verification because generated output adds `TelegramChatDiscoveryRequest` and `TelegramChatDiscoveryResponse`.
- `.gitea/workflows/ci.yml:56-60` sets `web=true` only for frontend/openapi/script/CI files.
- `.gitea/workflows/ci.yml:130-135` performs OpenAPI verification only in the web job.
- `.gitea/workflows/security.yml:71-79` runs `trivy fs`, not an image scan.
- `Directory.Build.props:7-8` treats warnings as errors but suppresses `NU1902` and `NU1903`.

## Expected outcome

The latest main commit must pass lint/type/test/audit/scan/container workflows. OpenAPI verification must run whenever backend API contracts can change. Security must include container image scanning. Dependency warnings should not be globally suppressed in a way that hides actionable audit failures.

## Fix guidance

Run `dotnet format` or fix MonitorCheckWorker indentation. Regenerate and commit OpenAPI/client artifacts. Make OpenAPI verification run when `src/`, `tests/`, contracts, or backend endpoint files change. Add an image build and Trivy image scan path. Revisit vulnerability warning suppression and rely on explicit audit gates.

## Acceptance criteria

- `ci.yml` succeeds on latest `main`.
- `docker-build.yml` completes successfully or is intentionally disabled with documented replacement.
- Backend API changes trigger OpenAPI export/client verification.
- Security workflow scans the built container image.
