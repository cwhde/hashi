# Hashi v2 spec compliance audit - F-series - 2026-06-04

Scope: fresh reread of `docs/implementation-spec/hashi-v2-implementation-spec.md`, review of prior audit issue/report formatting only for style, review of the current backend, frontend, workflow, deployment, DNS, monitoring, notification, resource, and Traefik implementation, and review of the latest public `ci.yml` and `docker-build-pulse.yml` failures linked from the public Actions page.

## Verification

- Public Actions page reviewed: https://git.juzo.io/juzo/hashi/actions
- `ci.yml #322` web job log reviewed for the OpenAPI contract failure: https://git.juzo.io/juzo/hashi/actions/runs/353/jobs/839/logs
- `docker-build-pulse.yml #323` native artifact job log reviewed for the unsupported artifact action failure: https://git.juzo.io/juzo/hashi/actions/runs/354/jobs/841/logs
- Current checkout reviewed at commit `31b9bbb4fdaf0a0259516855d087e7bf867fb720`.
- The public workflow failures were for older commit `52cdfc3481`; the current checkout already contains the E-series fixes for the stale OpenAPI/client artifacts and the unsupported Pulse artifact upload action.
- `dotnet test Hashi.slnx /p:SkipFrontendBuild=true` passed after the issue files were written: 342 tests passed, 0 failed.

## New Issues

- `F01-stream-public-port-confirmation-can-be-deleted-while-still-in-use.md` - shared TCP/UDP public-port confirmation is owned by the last synced resource, so deleting that resource can remove the entry point while another resource still uses the port.
- `F02-manual-dns-monitoring-is-not-selectable.md` - manual DNS records have dashboard flags but no monitoring opt-in/name, while monitoring provisioning creates a DNS endpoint for every enabled user-owned DNS record.
- `F03-status-ui-omits-required-detail-grouping-and-30-day-views.md` - the status admin surface lacks the required 30-day view, grouping/sorting controls, event timeline, uptime/min/max/avg detail, and endpoint settings workflow.
- `F04-notification-routing-ignores-routes-cooldowns-and-endpoint-overrides.md` - notification delivery ignores route configuration and sends monitor/security events to every enabled provider type without cooldowns, severity thresholds, endpoint overrides, or route-managed recovery controls.

## Notes

I did not re-file the latest public `ci.yml` OpenAPI artifact failure or `docker-build-pulse.yml` artifact upload failure. They are substantial failures in the public runs the user linked, but they were produced by older commit `52cdfc3481`; the current checkout has `dockerComposeSnippet` in both committed API artifacts and uses `actions/upload-artifact@v3` in the Pulse native artifact workflow.

I also did not re-file previous A-through-E issues while auditing the current tree. The issues below are new gaps found by comparing the current implementation to the spec rather than checking whether the old issues were fixed.
