# AUD-001 - Monitoring cannot configure allowed HTTP response codes

Priority: Medium (P2)

Category: Missing feature / monitoring correctness

Specification reference: `docs/implementation-spec/hashi-v2-implementation-spec.md`, section 24 (Settings → Monitoring: “Allowed HTTP codes”); section 18 (status monitoring).

## Problem

Monitoring settings let an operator set check interval and timeout, but do not let them choose which HTTP response codes count as healthy. `MonitorCheckWorker` treats every 2xx response as up and every other response as down through `HttpResponseMessage.IsSuccessStatusCode`. This prevents legitimate endpoints that use a configured 3xx, 401, 404, or other expected status from being represented accurately.

## Evidence

- `src/Hashi.Infrastructure/Platform/MonitorCheckWorker.cs` uses `response.IsSuccessStatusCode` in the HTTP/HTTPS/H2C check path.
- `AppSettingsEntity` and settings API contracts expose monitor interval and timeout, but no allowed-code setting (`src/Hashi.Infrastructure/Persistence/Entities/CoreEntities.cs`, `src/Hashi.Contracts/Api/PlatformContracts.cs`).
- The settings endpoint and frontend have no allowed-code field (`src/Hashi.Api/Features/Setup/SettingsEndpoints.cs`, `web/src/routes/(admin)/settings/+page.svelte`).
- The implementation specification explicitly lists “Allowed HTTP codes” under Monitoring settings.

## Expected outcome

Operators can configure the accepted HTTP status codes for checks, with a safe default matching current behavior. The worker uses the configured policy consistently, and invalid or empty configuration is rejected or falls back safely.

## Acceptance criteria

- Add persisted settings and API/UI controls for allowed status codes.
- Validate and normalize configured codes, including sensible limits and defaults.
- Apply the policy to HTTP, HTTPS, and H2C checks.
- Add unit coverage for default behavior, custom accepted codes, rejected codes, and malformed configuration.
- Preserve existing monitor behavior for installations that have no saved value.
