# AUD-157: Several management pages expose creation but omit required edit apply or lifecycle controls

Priority: P1

Category: Missing feature / management workflows

Status: open; confirmed by manual source trace.

## Finding

Connections has create/validate/list but no edit/delete workflow; AdGuard has no connection edit/delete/enable controls; notification providers/routes lack editing/enabled toggles; monitoring has no manual endpoint CRUD/configuration flow despite backend DTOs; Traefik lacks install/version/restart/health controls. DNS provider editing/deletion and a usable provider plan/apply action are also absent. These are functional management gaps, not the deferred visual redesign. Existing resource/firewall/script gaps remain separately AUD-091/048/148.

The security page also omits editing manual-entry scope/bypass/expiry/permanence and feed parser/refresh options, despite those being supported specification controls. Its firewall preview action discards the returned plan instead of presenting it for review. These operational controls need explicit usable flows rather than merely an existing backend record type.

## Evidence

web/src/routes/(admin)/connections/+page.svelte; adguard/+page.svelte; dns/+page.svelte; status/+page.svelte; traefik/+page.svelte; web/src/lib/components/notifications/; corresponding API/service methods were manually read. Some backend operations themselves remain defective in AUD-013/074/075/076/095.

## Expected outcome and acceptance criteria

Make every required supported lifecycle operation reachable with a complete validated UI flow, accurate result/error handling, reauthentication and reviewed destructive confirmation. Prioritize first installation and routine correction of a misconfigured connection before cosmetic redesign.
