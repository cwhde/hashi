# C05 - Public dashboard omits manual DNS tiles and required summary fields

Priority: High

Spec conflicts: section 20 requires the public dashboard to include resources and manual DNS entries selected for dashboard display, require display names for manually managed external DNS entries, link cards to public URLs, collapse search by default, and show both online-host and Linux-firewall-host availability counts.

## Problem

The public dashboard only lists resources. There is no API or data model path for manual DNS records to be selected for dashboard display with a required display name, even though the spec explicitly lists manual DNS entries as a dashboard source.

The current public dashboard also misses several required presentation details: search is always visible, the count is only `services online`, there is no Linux firewall host availability count, and cards without a resource domain link to `#` instead of a public URL.

## Evidence

- `docs/implementation-spec/hashi-v2-implementation-spec.md:1110-1129` lists resources and manual DNS entries as dashboard sources and defines display/count/link requirements.
- `src/Hashi.Api/Features/Platform/PlatformEndpoints.cs:340-349` builds `/api/public/apps` exclusively from resources.
- `src/Hashi.Infrastructure/Persistence/Entities/DnsEntities.cs:70-95` defines manual DNS records without dashboard visibility or display-name fields.
- `src/Hashi.Contracts/Api/DnsContracts.cs:28-44` exposes manual DNS create/update/response contracts without dashboard visibility or display-name fields.
- `web/src/lib/components/public/PublicDashboardView.svelte:46-60` renders the search input visibly by default.
- `web/src/lib/components/public/PublicDashboardView.svelte:41-43` shows only service online count.
- `web/src/lib/components/public/PublicDashboardView.svelte:69-76` links cards without a domain to `#`.

## Expected outcome

The public dashboard should be driven by a public dashboard item model that includes selected resources and selected manual DNS entries, with required display names for manual DNS entries and the summary counts required by the spec.

## Fix guidance

Add dashboard visibility and display name fields for manual DNS records, or introduce a dedicated dashboard item table that can reference resources and manual DNS records. Update the public app API to return safe public dashboard DTOs for both sources. Add firewall host availability summary data and make search collapsed by default in the UI.

## Acceptance criteria

- Manual DNS records can be selected for public dashboard display only after a display name is set.
- `/api/public/apps` includes selected manual DNS entries through a safe public DTO.
- Dashboard cards link to public URLs instead of `#`.
- Search is collapsed by default.
- The public dashboard shows both `x / n hosts online` and `x / n Linux firewall hosts available`.
- Tests cover manual DNS dashboard selection and public DTO rendering.
