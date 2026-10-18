# AUD-147: Duplicate monitor names break the public status keyed list

Priority: P2

Category: Frontend reliability / public status

Status: open; confirmed by manual source trace.

## Finding

PublicStatusView keys repeated rows by item.name. Monitor creation and provisioned resource names do not establish global uniqueness, so two enabled public endpoints may legitimately share a display name. Svelte rejects duplicate keys, preventing the status list from rendering correctly. Renaming an endpoint also unnecessarily replaces its identity.

## Evidence

web/src/lib/components/public/PublicStatusView.svelte:66; src/Hashi.Infrastructure/Platform/MonitoringService.cs:106-120 and public status projection.

## Expected outcome and acceptance criteria

Include and use a stable unique endpoint ID in the public DTO/list key while keeping display names independent. Two public monitors with the same name must render and update normally.
