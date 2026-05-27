# Hashi V2 Spec Compliance Audit - 2026-05-26

Audit branch: `audit/hashi-v2-spec-compliance-2026-05-26`

Audited commit: `af8c10833095f7a922731f83fa8a228ac95ab64d`

Spec source: `C:\Users\hideyoshi\Downloads\hashi-v2-implementation-spec.md`

Scope: reviewed the V2 implementation outside `hashi.old/`, including backend, frontend, tests, migrations, docs, deployment files, API contracts, and Gitea workflow state for the audited commit.

## Gitea issue status

The available repository token can read Actions and write repository contents, but the Gitea issue API rejects issue operations with:

`token does not have at least one of required scope(s), required=[read:issue], token scope=write:repository`

Because of that, this audit creates issue-ready Markdown files under `docs/audits/issues/` instead of opening live Gitea issues. Each file is structured as a Gitea issue body with evidence, expected outcome, and acceptance criteria.

## Latest workflow state

- `security.yml` run 182, API run id 212, job `scan` id 507: `success`.
- `docker-build.yml` run 181, API run id 211, job `build-and-push` id 506: `cancelled`.
- `ci.yml` run 180, API run id 210: `failure`.
- CI backend job id 503 failed during `dotnet format Hashi.slnx --verify-no-changes` with whitespace errors in `src/Hashi.Infrastructure/Platform/MonitorCheckWorker.cs` lines 151-233.
- CI web job id 504 failed `Verify OpenAPI contract` because `openapi/hashi.json` and `web/src/lib/api/schema.d.ts` are stale. The generated diff adds `TelegramChatDiscoveryRequest` and `TelegramChatDiscoveryResponse`.
- Local `dotnet test Hashi.slnx /p:SkipFrontendBuild=true` could not run on this machine because only .NET SDK 9.0.201 is installed locally and the repo targets `net10.0`.

## Issue index

| ID | Title | Priority |
| --- | --- | --- |
| A01 | Admin auth and CSRF middleware expose protected endpoints | Critical |
| A02 | Vault session and service-sync design break secret boundaries | Critical |
| A03 | DNS sync plan/apply endpoint flow is unusable | High |
| A04 | DNS ownership safety can modify unowned provider records | Critical |
| A05 | Edge SSO/OIDC fails open and bypasses standard token validation | Critical |
| A06 | Traefik validation, rendering, and apply safety do not meet spec | High |
| A07 | Firewall renderer/apply can open all public traffic and rollback successful applies | Critical |
| A08 | AdGuard writes bypass plan/preview/apply/audit flow | High |
| A09 | Setup completion and resource invariants miss required minimums | High |
| A10 | Public port routing depends on the private admin API and docs reverse ports | High |
| A11 | Privileged scripts lack the specified target, secret, output, and deployment model | High |
| A12 | Notification provider secrets are stored as plaintext JSON | Critical |
| A13 | Pulse install/token and DNS sync behavior are not safe enough | Medium |
| A14 | Database model omits important spec tables and operational fields | High |
| A15 | CI is red and workflow coverage does not satisfy the spec | High |
| A16 | Tests miss critical endpoint and safety behavior | High |

## Recommended fix workflow

Do not create empty branches for all issues now. For each fix, branch from the current `main` when work starts:

`fix/A01-admin-auth-csrf`

If live Gitea issues are created later, replace `A01` with the issue number:

`fix/123-admin-auth-csrf`

This keeps each fix branch fresh and avoids a pile of stale branch heads.
