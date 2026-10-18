# AUD-145: Public dashboards select their mode from browser ports and fail behind HTTPS proxies

Priority: P1

Category: Missing implementation / public app routing

Status: open; confirmed by manual source trace.

## Finding

The backend serves the public dashboard/status from dedicated local ports, but the shared static frontend chooses its root view by window.location.port. A normal HTTPS reverse proxy maps443 to backend8081/8082; the browser sees an empty/default443 port and renders AdminAppShell instead of the intended public view. Runtime config supplies backend port numbers only and cannot identify which local listener served the request. Public API restrictions then prevent the rendered admin shell from working. This does not expose admin API access on the public listener.

## Evidence

web/src/lib/public/port-mode.ts:17-35; web/src/routes/+page.svelte; src/Hashi.Api/Program.cs:127-138; Hosting/PublicPortRoutingMiddleware.cs. The root-only public-port product behavior must survive ordinary TLS termination.

## Expected outcome and acceptance criteria

Emit the server-known surface mode for the local listener or use another explicit validated public surface configuration. Dashboard/status through separate HTTPS hostnames on443 must render their intended views while keeping admin API isolation.
