# C04 - Public app API exposes internal resource details

Priority: Critical

Spec conflicts: public pages can be enabled or disabled and must expose only selected records and health summaries. The public dashboard must have no admin controls and must be generated from selected entries.

## Problem

`/api/public/apps` is anonymous and returns the full admin `ResourceResponse` shape for every dashboard-enabled resource. That payload includes internal target host/port, firewall and Pulse identifiers, routes, resource rules, extra middlewares, and auth/WAF configuration. The public dashboard component even renders the internal target host and port when a domain is absent.

The public dashboard enabled flag is enforced only by the dedicated public dashboard port middleware. The same anonymous `/api/public/apps` endpoint remains available on the admin port because `/api/public` is always treated as public by admin auth middleware and the endpoint itself does not check `PublicDashboardEnabled`.

## Evidence

- `docs/implementation-spec/hashi-v2-implementation-spec.md:429-433` says public pages can be disabled and expose only selected records and health summaries.
- `docs/implementation-spec/hashi-v2-implementation-spec.md:1110-1129` defines the public app dashboard as a selected-entry public view with no admin controls.
- `src/Hashi.Contracts/Api/PlatformContracts.cs:3-25` defines `ResourceResponse` with internal target fields, firewall/Pulse ids, auth/WAF settings, routes, and rules.
- `src/Hashi.Api/Features/Platform/PlatformEndpoints.cs:340-349` returns `ResourceResponse` objects from `/api/public/apps`.
- `src/Hashi.Api/Features/Platform/PlatformEndpoints.cs:351-353` explicitly allows anonymous access to that endpoint.
- `src/Hashi.Api/Hosting/AdminApiAuthMiddleware.cs:14-18` marks `/api/public` as always public on any port.
- `src/Hashi.Api/Hosting/PublicPortRoutingMiddleware.cs:23-30` checks `PublicDashboardEnabled` only when the request is on the configured public dashboard port.
- `web/src/lib/components/public/PublicDashboardView.svelte:73-76` displays `${app.targetHost}:${app.targetPort}` when no public domain exists.

## Expected outcome

Public app APIs should return a purpose-built public DTO containing only safe public display data, and public dashboard data should not be served when the dashboard is disabled.

## Fix guidance

Create a `PublicAppResponse` that contains only public display name, public URL/domain, public health/status summary, and optional safe presentation metadata. Make `/api/public/apps` check `PublicDashboardEnabled` regardless of port, or split admin preview from public API. Avoid returning internal target information, routes, rule definitions, middleware names, firewall ids, and Pulse ids to anonymous callers.

## Acceptance criteria

- Anonymous `/api/public/apps` does not return internal target hosts, target ports, firewall ids, Pulse ids, routes, rules, auth policy, WAF mode, or middleware names.
- `/api/public/apps` returns 404 or an empty disabled response when `PublicDashboardEnabled` is false on every port.
- The public dashboard never renders internal host:port fallback values.
- Tests cover anonymous public API payload shape and disabled-dashboard behavior on both admin and public dashboard ports.
