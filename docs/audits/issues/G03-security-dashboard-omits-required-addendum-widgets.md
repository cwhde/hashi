# G03 - Security dashboard omits required addendum widgets

Priority: Medium

Spec conflicts: addendum section 11.2

## Problem

The addendum lists specific security dashboard widgets that must be present. The current dashboard implements a smaller set: request counts, WAF counts, top blocked IPs, countries/ASNs, top challenged/blocked resources, recent events, firewall IP block count, blocklist entry count, and total security event count.

Several required widgets are missing from the API contract and UI: top challenged IPs, recent manual actions, blocklist matches over time, CAPTCHA solved/failed/ignored, active soft blocks, stale blocklist sources, and stale GeoIP/ASN database status. Because the DTO does not include these fields, the frontend cannot render them.

## Evidence

- `docs/implementation-spec/hashi-v2-implementation-spec-addendum.md:1001` through `docs/implementation-spec/hashi-v2-implementation-spec-addendum.md:1013` requires widgets for top challenged IPs, top blocked IPs, recent manual actions, blocklist matches over time, CAPTCHA solved/failed/ignored, active soft/firewall blocks, stale blocklist sources, and stale GeoIP/ASN database status.
- `src/Hashi.Contracts/Api/PlatformContracts.cs:580` through `src/Hashi.Contracts/Api/PlatformContracts.cs:600` defines `SecurityDashboardResponse` without fields for most of those required widgets.
- `src/Hashi.Infrastructure/Platform/SecurityIngestionService.cs:370` starts the dashboard builder, and `src/Hashi.Infrastructure/Platform/SecurityIngestionService.cs:581` through `src/Hashi.Infrastructure/Platform/SecurityIngestionService.cs:601` returns only the reduced DTO shape.
- `web/src/routes/(admin)/security/+page.svelte:585` through `web/src/routes/(admin)/security/+page.svelte:622` renders Allowed, Blocked, Challenged, WAF, Active blocks, Top blocked IPs, Top challenged/blocked resources, and Signals widgets, but not the missing addendum widgets.

## Expected outcome

The security dashboard should expose and render all widgets required by the addendum, either as direct widgets or clearly equivalent representations with the same operational information.

## Fix guidance

Extend `SecurityDashboardResponse` and `SecurityIngestionService.GetDashboardAsync` with the missing dashboard data. Add frontend widgets for top challenged IPs, recent manual actions, blocklist match trend data, CAPTCHA outcome counts, current active soft blocks, stale blocklist sources, and stale GeoIP/ASN database status.

## Acceptance criteria

- Dashboard API includes fields for each widget listed in addendum section 11.2.
- The security dashboard renders those widgets.
- Widget queries respect the same time/filter inputs as the rest of the dashboard where applicable.
- Tests cover at least the API shape and representative aggregation for the newly added widgets.
