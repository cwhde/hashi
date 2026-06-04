# Hashi V2 Addendum Implementation Prep

Baseline reviewed on 2026-06-04 after `git pull --ff-only`.

- Current commit: `a89445c96441c7a4209ef18b24dcc65d5319101c` (`merge audit series F fixes`).
- Original spec read first: `docs/implementation-spec/hashi-v2-implementation-spec.md`.
- Addendum read second: `C:\Users\hideyoshi\Downloads\Hashi V2 Addendum.md`.
- Audit context reviewed: `docs/audits/` and `docs/audits/issues/`.

This folder is outside `docs/audits/` on purpose. It is implementation prep for the addendum, not another compliance audit.

## Current Repo Shape

- Backend: .NET 10 Minimal APIs under `src/Hashi.Api`, domain helpers under `src/Hashi.Core`, EF/PostgreSQL and services under `src/Hashi.Infrastructure`.
- Frontend: SvelteKit 5 under `web/src`, generated API types in `web/src/lib/api/schema.d.ts` and `web/src/lib/api/types.ts`.
- Pulse: Go agent under `agents/pulse`.
- Deployment and CI: `deploy/docker/Dockerfile`, `agents/pulse/Dockerfile`, `hashi.old/docker/Dockerfile`, and `.gitea/workflows/*.yml`.
- OpenAPI: committed contract at `openapi/hashi.json`, regenerated with `scripts/export-openapi.sh` and `scripts/generate-api-client.sh`.

## Existing Partial Support

The addendum should build on these existing pieces:

- `SecurityIngestionService` already ingests access-log and forward-auth events, updates `AccessLogEventEntity`, `SecurityEventEntity`, `AbuseBucketEntity`, and `SecurityRequestBucketEntity`, and can sync IP block entries to firewall hosts.
- `EdgeAuthService` already evaluates resource rules, manual-ish allow subjects, blocklist entries, OIDC edge sessions, and adaptive states.
- `FirewallApplyService` and `FirewallScriptRenderer` already render Hashi-owned chains/sets and include active IP block entries in `hashi_blocked`.
- `PulseAgentService` already stores heartbeat metadata, selected/public/private IPs, and queues DNS sync for Pulse-backed resources.
- `AdGuardSyncService` already owns preview/apply/result flow for Hashi-managed rewrites and preserves manual rewrites.
- `BackgroundJobService` already tracks job status, last run, next run, duration, diff summary, and errors.
- Admin auth, CSRF, and recent reauthentication already exist in `AdminApiAuthMiddleware`, `AdminCsrfMiddleware`, and `ReauthenticationState`.

## Major Gaps Against Addendum

- No normalized `security_subjects` or rich `security_subject_states` table.
- Manual allow/block is split between `FirewallAllowedSubjectEntity` and manual `BlocklistEntryEntity`; it does not have addendum bypass flags or scopes.
- "Challenge" currently means OIDC redirect/auth challenge; there is no Cap CAPTCHA provider, challenge page, challenge state lifecycle, or request-while-challenged escalation.
- `blocklist_entries` exists, but there are no blocklist sources, fetch runs, SSRF-safe fetcher, parser, preview, or per-source settings.
- Connection targets are mostly static host/base URL fields; there is no reusable static-host/static-IP/Pulse-agent target model.
- Internal agent DNS under `hashi.home.arpa` is absent.
- Setup/settings UI does not expose Cap, individual blocklist selection, or internal agent DNS.
- Main and Pulse Dockerfiles are not cross-build aware, and workflows have limited dependency/tool caching.

## Audit Context

The audits in `docs/audits/` are useful historical guardrails. Current `main` includes many fixes from A through F series, but the issue files remain useful evidence for safety boundaries:

- Preserve DNS, AdGuard, firewall, Traefik, and sync plan/apply ownership rules from the original spec.
- Keep public/admin API separation and safe public DTOs.
- Keep forwarded client context handling when making forward-auth decisions.
- Keep GeoIP availability validation for country/region/ASN rules.
- Keep OpenAPI and frontend generated types committed cleanly after API work.
- Keep Gitea workflow compatibility; do not assume GitHub-only features.

## Recommended Order

0. `RESEARCH-RESOURCES.md`
1. `TASK-01-data-model-and-contract-foundation.md`
2. `TASK-02-security-decision-engine.md`
3. `TASK-03-subject-search-manual-actions-ui.md`
4. `TASK-04-cap-captcha-challenge-flow.md`
5. `TASK-05-blocklist-source-management.md`
6. `TASK-06-agent-bound-connections.md`
7. `TASK-07-internal-agent-dns.md`
8. `TASK-08-setup-settings-and-background-jobs.md`
9. `TASK-09-ci-cd-optimization.md`
10. `TASK-10-cross-feature-acceptance.md`

Tasks 04, 05, 06, and 07 can start after the data model foundation is merged, but task 02 should land before task 03 and before the final CAPTCHA enforcement pass.
