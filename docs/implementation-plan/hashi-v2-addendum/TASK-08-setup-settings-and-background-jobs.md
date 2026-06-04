# TASK-08: Setup, Settings, and Background Jobs

## Goal

Wire the addendum features into setup, settings, recurring jobs, auth/reauth policy, activity visibility, and generated contracts.

This is the integration task that turns backend feature pieces into coherent product workflows.

## Spec Context

- Original spec sections: 7, 21, 24, 25, 27, 30, 31.
- Addendum sections: 11.1, 12, 13, 16.2, 16.3, 18.

## Current Code Anchors

- Setup UI: `web/src/lib/components/setup/steps/OptionalStep.svelte`
- Setup step registry: `web/src/lib/setup/steps.ts`, `src/Hashi.Core/Setup/SetupStep.cs`
- Setup endpoints: `src/Hashi.Api/Features/Setup/SetupEndpoints.cs`, `SetupAdvanceEndpoints.cs`
- Settings UI: `web/src/routes/(admin)/settings/+page.svelte`
- Settings endpoints: `SettingsEndpoints` in `SetupAdvanceEndpoints.cs`
- Background jobs: `BackgroundJobService`, hosted workers under `src/Hashi.Infrastructure/Platform/*Worker.cs`
- Activity UI/API: `web/src/routes/(admin)/activity/+page.svelte`, `ActivityEndpoints`
- Auth/CSRF/reauth: `AdminApiAuthMiddleware.cs`, `AdminCsrfMiddleware.cs`
- API generation: `scripts/export-openapi.sh`, `scripts/generate-api-client.sh`

## Setup Additions

Add optional setup panels:

1. Cap CAPTCHA integration.
2. Recommended blocklist selection.
3. Internal agent DNS domain configuration.

Cap setup fields:

- Enable CAPTCHA integration.
- Cap challenge base URL.
- Site key.
- Secret key.
- Test verification.
- Public challenge resource domain.
- Optional Cap admin resource domain.
- Recommended SSO for admin resource.

Blocklist setup fields:

- Individual recommended feed checkboxes.
- Custom URL option.
- Preview parsed entries before enabling.
- Enforcement mode.

Internal DNS setup fields:

- Enable internal agent DNS.
- Domain default `hashi.home.arpa`.
- Warning that this is DNS-only, not reverse proxy.
- Requires AdGuard Home connection.

## Settings Additions

Add settings panels for:

- CAPTCHA.
- Challenge/ban/escalation policy.
- Blocklists and source health.
- Internal agent DNS.
- Security subject defaults.

Keep high-risk fields behind reauthentication where required.

## Background Jobs

Register jobs in `BackgroundJobKeys` and `EnsureJobsAsync`:

- `blocklist-fetch`
- `security-bucket-aggregation`
- `block-expiry`
- `internal-agent-dns-sync`
- `challenge-cleanup`

Implement hosted workers or fold into existing services only if ownership remains clear.

Each job must:

- Update `background_jobs`.
- Record last run, next run, status, duration, diff summary, and error.
- Avoid hard failing startup if optional dependencies are absent.
- Use service-sync secrets where needed.

## Auth and Audit Integration

Add recent reauthentication requirements for:

- Manual permanent block.
- Firewall block.
- Blocklist enable with firewall enforcement.
- Cap integration changes.
- Cap secret rotation.
- Internal DNS domain change.
- Deleting required challenge resource after disabling CAPTCHA.

Audit events required for:

- Manual allow/block create/update/delete.
- Firewall block apply/remove.
- Blocklist source create/update/delete/enable/disable.
- CAPTCHA settings changes.
- Cap secret rotation.
- Internal DNS domain change.
- Agent-bound connection target changes.
- Any high-risk sync apply.

## Contract Hygiene

Every API change must regenerate:

- `openapi/hashi.json`
- `web/src/lib/api/schema.d.ts`

Update typed client helpers in:

- `web/src/lib/api/client.ts`
- `web/src/lib/api/types.ts` if local aliases are needed.

## Tests

- Setup can save/skip each optional addendum panel.
- Settings read/update round trips.
- Background jobs are registered and visible in Activity.
- Reauth middleware matches new high-risk paths.
- Audit events are written for required mutations.
- OpenAPI/client artifact diff is clean.

## Acceptance

- Addendum features are discoverable during setup and configurable after setup.
- Routine jobs run without an active browser session.
- High-risk operations require recent passkey reauthentication.
