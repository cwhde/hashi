# A10 - Public port routing depends on the private admin API and docs reverse ports

Priority: High

Spec conflicts: public page behavior in sections 9.4 and 18. Public ports should serve selected public app/status data without exposing admin API surfaces.

## Problem

The runtime maps public dashboard to port 8081 and public status to port 8082, but operational docs reverse those meanings in some places. More importantly, public ports intentionally return 404 for `/api/*`, while the frontend public pages switch API calls back to the admin port 8080 when loaded on 8081/8082.

That means public pages depend on the admin API port being reachable from public clients. The hardening docs recommend restricting admin API port 8080 to management networks, which would make public pages fail or show empty data in the hardened deployment the spec wants.

## Evidence

- `src/Hashi.Api/Hosting/HashiPorts.cs:5-7` sets admin 8080, dashboard 8081, status 8082.
- `src/Hashi.Api/Hosting/PublicPortRoutingMiddleware.cs:6-9` documents dashboard 8081 and status 8082.
- `src/Hashi.Api/Hosting/PublicPortRoutingMiddleware.cs:17-20` blocks API/OpenAPI on public ports.
- `web/src/lib/api/base-url.ts:7-10` rewrites API calls from public ports to admin port 8080.
- `docs/operations/backup-restore.md:21` says status is 8081 and app dashboard is 8082.
- `docs/operations/hardening.md:6-7` recommends restricting admin API 8080 while public ports expose data.

## Expected outcome

Public ports should serve public app/status data without requiring browser access to the admin API port. Docs must consistently state 8081 dashboard and 8082 status, or code must be changed to match a different intended mapping.

## Fix guidance

Serve public read-only API data from the public ports under a narrow public route, or render public data server-side into the SPA shell. Keep admin routes unavailable on public ports. Fix docs to match runtime constants.

## Acceptance criteria

- Public dashboard/status pages work when 8080 is inaccessible to the client.
- Public ports still do not expose admin APIs.
- Docs consistently name 8081 as dashboard and 8082 as status, or all code/docs are consistently changed.
- Playwright/e2e covers actual public-port mode and data fetch.
