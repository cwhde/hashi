# AUD-038: Public dashboard does not associate manual DNS entries with their monitor

Priority: P2 — selected manual DNS entries can be shown Online while their monitor is down

Category: Public dashboard / monitoring correctness

Spec reference: `docs/implementation-spec/hashi-v2-implementation-spec.md` §§18.1 and 20. Manual DNS entries are monitor sources, and dashboard-enabled manual DNS entries appear in the public dashboard.

## Finding

`MonitoringService.SyncInfrastructureEndpointsAsync` provisions a manual DNS monitor with `Name = MonitoringDisplayName`. `PublicDashboardService.GetAsync` searches for that record's monitor by requiring `Name == "DNS: {record.Name}"` instead. The lookup therefore misses the provisioned monitor whenever it exists under the configured display name. `ResolveStatus(null, true)` defaults to `Online`, so a down or degraded monitored DNS entry is presented as Online and its latency is omitted.

## Evidence

- `src/Hashi.Infrastructure/Platform/MonitoringService.cs`: manual DNS monitor endpoint name is the record's trimmed `MonitoringDisplayName`.
- `src/Hashi.Infrastructure/Platform/PublicDashboardService.cs`: manual DNS dashboard item monitor lookup uses `DNS: {record.Name}` and defaults missing monitor status to Online.

## Expected outcome

Associate the monitor and DNS record by their stable `DnsRecordId` relationship, not by a display-name convention. Missing monitor data should not imply a verified healthy status.

## Acceptance criteria

- Dashboard-enabled manual DNS items show the status and latency of their own monitor endpoint.
- Renaming a monitoring display name does not break the association.
- A missing or never-checked monitor is not silently reported as Online.
