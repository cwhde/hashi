# AUD-013: Hetzner DNS provider uses the shut-down legacy API

Priority: P1 — core integration unavailable

Category: Wrong implementation / provider integration

Spec reference: implementation spec §§8.1, 8.3, 24 and 27; the spec requires a working Hetzner DNS adapter for zone discovery, record operations, and DNS challenge use.

## Finding

The active adapter is configured for `https://dns.hetzner.com/api/v1/` and uses the legacy individual-record endpoints (`zones`, `zones/{id}/records`, `records`, `records/{id}`) with the legacy `Auth-API-Token` header and response shapes. Hetzner shut down the old DNS Console and API in May 2026, redirecting `dns.hetzner.com` to the Console. The current provider therefore cannot validate credentials, list zones, or manage records against the provider service.

Hetzner's replacement DNS integration is in the Cloud API and manages records through RRSets. Hashi's current implementation is structurally incompatible with that API: host, authentication, routes, request/response models, and record update semantics differ. Existing legacy tokens may also need replacement with project-scoped Cloud API tokens; that migration behavior must be reflected in the UI and documentation.

## Evidence

- `src/Hashi.Infrastructure/DependencyInjection.cs` sets the `hetzner-dns` base address to `https://dns.hetzner.com/api/v1/`.
- `src/Hashi.Infrastructure/Providers/Dns/HetznerDnsProvider.cs` calls the legacy `/zones`, `/zones/{zoneId}/records`, `/records`, and `/records/{recordId}` endpoints and sends `Auth-API-Token`.
- Hetzner's [official shutdown notice](https://status.hetzner.com/incident/c2146c42-6dd2-4454-916a-19f07e0e5a44) says the DNS Console and API were removed on 27 May 2026 and `dns.hetzner.com` redirects to `console.hetzner.com`.
- Hetzner's [current Cloud API reference](https://docs.hetzner.cloud/reference/cloud) documents DNS zones and RRSet operations under the Cloud API.

## Impact

Hetzner DNS connection validation and CRUD are unavailable for new installations. Any features relying on the adapter—including DNS planning/apply and Hetzner DNS-01 certificate setup—cannot work with the current provider backend.

## Expected outcome

Replace the adapter with the current Hetzner Cloud DNS API, support RRSet semantics and project-scoped credentials, and provide a clear migration path for existing stored connections. Preserve Hashi's record-level model through a correct mapping where possible, and report unsupported or ambiguous multi-record operations rather than silently overwriting unrelated records.

## Acceptance criteria

- Zone discovery and token validation use the current Cloud API and its authentication scheme.
- List, create, update, and delete correctly map Hashi record operations to RRSet operations, including multi-value RRsets and TTL handling.
- DNS-01 challenge create/cleanup works with the new API.
- Existing connections fail with an actionable migration message; new credential instructions explain the required token scope and project.
- Provider request/response handling is checked against the current official API contract, with integration verification using a disposable zone/token before release.
