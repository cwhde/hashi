# E01 - OpenAPI and frontend contract artifacts are not generator clean

Priority: High

Spec conflicts: section 29 requires generated OpenAPI and frontend types to be kept updated in the same change. Section 30 requires CI to generate OpenAPI and verify committed client types when committed.

## Problem

The current main branch has committed API artifacts that do not match the repository's own generation scripts. The latest `ci.yml` web job reaches the OpenAPI verification step after frontend check, lint, test, and build succeed, then fails because regenerating `openapi/hashi.json` and `web/src/lib/api/schema.d.ts` produces a diff.

This is not just formatting drift. The committed OpenAPI document still exposes the old Pulse install response property name, while the current backend contract and frontend types use the new Compose-snippet property. The same generator run also reorders `SecurityTopBlockedIpItem` and reports a missing final newline in `openapi/hashi.json`.

## Evidence

- `docs/implementation-spec/hashi-v2-implementation-spec.md:1561` requires generated OpenAPI and frontend types to be updated in the same change.
- `docs/implementation-spec/hashi-v2-implementation-spec.md:1581` requires CI to generate OpenAPI and verify committed client types if committed.
- `.gitea/workflows/ci.yml:136-141` runs `scripts/export-openapi.sh`, `scripts/generate-api-client.sh`, and `git diff --exit-code openapi/hashi.json web/src/lib/api/schema.d.ts`.
- The latest public `ci.yml #322` web job for commit `52cdfc3481` fails at that verification step: https://git.juzo.io/juzo/hashi/actions/runs/353/jobs/839/logs
- `openapi/hashi.json:7257-7268` still requires and exposes `dockerRunCommand` on `PulseInstallResponse`.
- `src/Hashi.Contracts/Api/PlatformContracts.cs:476` defines `PulseInstallResponse(string LinuxInstallScript, string DockerComposeSnippet)`.
- `web/src/lib/api/schema.d.ts:6126-6128` already exposes `dockerComposeSnippet`, so the committed API artifacts disagree with each other.
- The same workflow log shows regenerated output moving `SecurityTopBlockedIpItem` and ending with `\ No newline at end of file` for `openapi/hashi.json`.

## Expected outcome

The committed OpenAPI file and generated TypeScript schema should be in the exact state produced by the repository's generation scripts, and CI should not fail on the OpenAPI contract check.

## Fix guidance

Run the OpenAPI export and client generation scripts from a clean checkout and commit the resulting artifacts. If the generator output is nondeterministic because of schema ordering or final-newline behavior, stabilize the export step instead of hand-editing the generated files.

## Acceptance criteria

- `scripts/export-openapi.sh && scripts/generate-api-client.sh && git diff --exit-code openapi/hashi.json web/src/lib/api/schema.d.ts` passes on a clean Linux checkout.
- `PulseInstallResponse` uses the same property names in backend contracts, `openapi/hashi.json`, generated TypeScript, and frontend call sites.
- The latest `ci.yml` web job passes the OpenAPI verification step.
- A regression test or CI-only check continues to catch stale committed API artifacts.
