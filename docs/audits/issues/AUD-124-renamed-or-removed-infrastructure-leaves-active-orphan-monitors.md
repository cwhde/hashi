# AUD-124: Renamed or removed infrastructure leaves active orphan monitors

Priority: P2

Category: Reliability / monitoring ownership

Status: open; confirmed by manual source trace.

## Finding

Infrastructure monitor upsert uses display Name to identify endpoints that have no ResourceId or DnsRecordId. Renaming an infrastructure object creates a new monitor and leaves the old enabled row; removal similarly has no infrastructure orphan reconciliation. A manual monitor with the same name can be selected and overwritten by the provisioning upsert, or duplicate matching names make SingleOrDefault throw and abort provisioning/checks. The managed-field guard protects only ResourceId/DnsRecordId sources, so infrastructure-generated rows are editable as manual endpoints.

## Evidence

src/Hashi.Infrastructure/Platform/MonitoringService.cs:134-140, infrastructure provisioning and UpsertMonitorEndpoint at530-559; DisableOrphanedDnsMonitorEndpoints only handles DNS ownership. MonitorCheckWorker.cs:76 provisions before every check cycle.

## Expected outcome and acceptance criteria

Assign stable source kind/ID ownership to every generated monitor, reconcile removed sources, and enforce managed-field ownership consistently. Rename/delete and manual-name collision scenarios must not create stale monitors or abort all checks.
