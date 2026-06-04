# F02 - Manual DNS monitoring is not selectable

Priority: Medium

Spec conflicts: section 18.1 says monitor endpoints are created from manual DNS entries with monitoring enabled, and that a name is required for any monitored manual DNS entry.

## Problem

Manual DNS records have dashboard visibility fields, but they do not have any monitoring opt-in or monitoring display-name fields. The monitoring sync then creates an enabled DNS monitor endpoint for every enabled user-owned DNS record.

That means operators cannot choose which manual DNS records should become monitored status endpoints. It also collapses the spec's required monitored-manual-DNS name into an automatic `DNS: {record.Name}` label, so a record can become a monitor without the user intentionally naming or enabling it for monitoring.

## Evidence

- `docs/implementation-spec/hashi-v2-implementation-spec.md:971-979` limits this source to manual DNS entries with monitoring enabled and requires a name for monitored manual DNS entries.
- `src/Hashi.Infrastructure/Persistence/Entities/DnsEntities.cs:70-99` defines `DnsRecordEntity` with `Enabled`, `DashboardEnabled`, and `DashboardDisplayName`, but no monitoring opt-in or monitoring display-name fields.
- `src/Hashi.Contracts/Api/DnsContracts.cs:28-48` exposes DNS record create/update/response contracts with dashboard fields only.
- `src/Hashi.Infrastructure/Platform/MonitoringService.cs:355-367` selects every enabled user-owned DNS record and unconditionally provisions an enabled DNS endpoint named `DNS: {record.Name}`.

## Expected outcome

Manual DNS entries should create monitor endpoints only when monitoring/status is explicitly enabled for that DNS record. Enabling monitoring for a manual DNS entry should require a user-facing monitor name, separate from dashboard tile naming.

## Fix guidance

Add monitoring/status fields to manual DNS persistence and API contracts, for example `MonitoringEnabled` and `MonitoringDisplayName`. Validate that the display name is present when monitoring is enabled. Update the DNS UI so dashboard visibility and monitoring visibility are separate controls. Update monitoring provisioning to include only opted-in manual DNS records and to use the configured monitor name.

## Acceptance criteria

- Manual DNS records can be created and updated without becoming monitor endpoints by default.
- Enabling monitoring for a manual DNS record requires a non-empty monitor display name.
- Monitoring provisioning creates DNS endpoints only for opted-in manual DNS records.
- Disabling monitoring for a manual DNS record removes or disables the provisioned DNS monitor endpoint.
- Tests cover both non-monitored and monitored manual DNS records.
