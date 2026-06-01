# C06 - Public status publishes all enabled monitors

Priority: High

Spec conflicts: public pages must expose only selected data, and the status page public port can be enabled or disabled independently.

## Problem

The public status API returns every enabled monitor endpoint. There is no public/private status visibility flag on monitor endpoints, so internal monitors created for firewall hosts, Traefik hosts, AdGuard connections, Hashi itself, manual DNS records, or user-created endpoints become public as soon as they are enabled.

Like the public app endpoint, public status endpoints are anonymous on the admin port because `/api/public` is globally public. The `PublicStatusEnabled` flag is only enforced by the dedicated public status port middleware, not by the endpoints themselves.

## Evidence

- `docs/implementation-spec/hashi-v2-implementation-spec.md:429-433` requires public pages to expose only selected records and health summaries.
- `docs/implementation-spec/hashi-v2-implementation-spec.md:1790-1803` requires public status support and selected public data.
- `src/Hashi.Infrastructure/Persistence/Entities/PlatformEntities.cs:125-144` defines monitor endpoints with `Enabled` but no public visibility field.
- `src/Hashi.Contracts/Api/PlatformContracts.cs:415-435` exposes monitor endpoint contracts with `Enabled` but no public visibility field.
- `src/Hashi.Infrastructure/Platform/MonitoringService.cs:191-230` returns all enabled monitor endpoints from `PublicStatusAsync`.
- `src/Hashi.Infrastructure/Platform/MonitoringService.cs:233-235` builds public summary counts from all enabled monitor endpoints.
- `src/Hashi.Api/Hosting/AdminApiAuthMiddleware.cs:14-18` marks all `/api/public` endpoints anonymous on every port.
- `src/Hashi.Api/Hosting/PublicPortRoutingMiddleware.cs:51-58` checks `PublicStatusEnabled` only on the public status port.

## Expected outcome

Only endpoints explicitly selected for public status should appear in public status API responses. Disabling the public status page should disable public status data on every port.

## Fix guidance

Add a public visibility flag or a dedicated public status selection model. Use that selection in `PublicStatusAsync` and `PublicSummaryAsync`. Enforce `PublicStatusEnabled` in the endpoint/service path as well as the public-port middleware, or split admin preview from public API.

## Acceptance criteria

- Monitor endpoints can be enabled for monitoring without being public.
- Anonymous `/api/public/status` and `/api/public/status/summary` include only public-selected endpoints.
- Public status endpoints return 404 or a disabled response when `PublicStatusEnabled` is false on every port.
- Tests cover private enabled monitors, selected public monitors, and disabled public status on both admin and public status ports.
