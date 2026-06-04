# F03 - Status UI omits required detail, grouping, and 30-day views

Priority: Medium

Spec conflicts: sections 18.4 and 18.5 require 60-minute, 1-hour, 24-hour, 7-day, 30-day, and event-timeline views, plus a status landing page with grouping/sorting and a detail page with uptime, min/max/avg response time, incident timeline, and endpoint settings.

## Problem

The admin status page currently provides a flat endpoint table, search, a public-status toggle, a 60-minute strip, and a latency chart for the selected range. It does not expose the required 30-day range, grouping controls, sorting controls, event timeline, uptime stats, response-time min/max/avg, or endpoint settings detail workflow.

The backend exposes endpoint, rollup, and event lists, but the frontend only loads endpoints and rollups. As implemented, the status surface is useful as a lightweight monitor list, but it does not meet the required operational status workflow from the spec.

## Evidence

- `docs/implementation-spec/hashi-v2-implementation-spec.md:1029-1036` requires the 60-minute bar, 1-hour, 24-hour, 7-day, 30-day, and event timeline views.
- `docs/implementation-spec/hashi-v2-implementation-spec.md:1040-1057` requires grouping, sorting, group display, status detail stats, incident/event timeline, and endpoint settings.
- `web/src/routes/(admin)/status/+page.svelte:21-28` stores endpoints, rollups, search, selection, and a selected hour range, but no group, sort, events, detail stats, or endpoint settings state.
- `web/src/routes/(admin)/status/+page.svelte:34-43` loads only `api.listStatusEndpoints()` and `api.listStatusRollups(...)`.
- `web/src/routes/(admin)/status/+page.svelte:113-124` offers only last hour, last 24 hours, and last 7 days.
- `web/src/routes/(admin)/status/+page.svelte:142-181` renders a flat table without group-by or sort controls.
- `web/src/routes/(admin)/status/+page.svelte:185-191` renders only the selected endpoint latency chart.
- `src/Hashi.Api/Features/Platform/PlatformEndpoints.cs:297-321` exposes rollups and events, but the page does not call `api.listStatusEvents`.

## Expected outcome

The status UI should support the complete required status workflow: range selection through 30 days, event timeline, group and sort modes, a detail view with current status, last check, min/max/avg response time, uptime stats, latency graph, incident/event timeline, and endpoint settings.

## Fix guidance

Extend the status page state and API usage to load events and aggregate stats for the selected endpoint/range. Add group-by controls for host, Linux firewall host, status, and resource type, and sort controls for name, state, latency, uptime, and last event. Add a 30-day option and a detail panel or route that exposes endpoint settings and required status statistics.

If the backend cannot currently compute a required statistic efficiently, add a dedicated summary/detail endpoint rather than forcing the frontend to infer everything from raw rollups.

## Acceptance criteria

- The status UI exposes last 1 hour, 24 hours, 7 days, and 30 days latency/uptime views plus the last 60 minutes strip.
- Operators can group endpoints by host, Linux firewall host, status, and resource type.
- Operators can sort endpoints by name, state, latency, uptime, and last event.
- The selected endpoint detail shows current status, last check, min/max/avg response time, uptime stats, latency graph, incident/event timeline, and endpoint settings.
- Frontend tests cover range, grouping, sorting, event timeline, and endpoint-settings behavior.
