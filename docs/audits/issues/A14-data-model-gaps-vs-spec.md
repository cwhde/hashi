# A14 - Database model omits important spec tables and operational fields

Priority: High

Spec conflicts: data model sections 26 and 27 plus non-negotiable rules 1, 10, 11, 12, 20, and 22.

## Problem

The EF model implements many tables, but several spec-level concepts are either missing or collapsed into generic JSON fields without enough operational state to enforce ownership, run history, background job state, or safety invariants.

Some omissions directly contribute to other issues: DNS ownership is not separate enough to protect unowned records, scripts lack run/output tables, notification delivery history is absent, and firewall rules lack first-class allowed/blocked/port/subnet/generated-script tables.

## Evidence

- `src/Hashi.Infrastructure/Persistence/HashiDbContext.cs:8-83` lists current DbSets. Missing spec tables include `connection_health`, `dns_record_ownership`, `resource_targets`, `resource_ports`, `system_resources`, `firewall_subnets`, `firewall_ports`, `firewall_allowed_subjects`, `firewall_block_subjects`, `firewall_generated_scripts`, `security_subjects`, `geoip_databases`, `security_dashboard_snapshots`, `notification_routes`, `notification_deliveries`, `pulse_heartbeats`, `pulse_tokens`, `host_script_targets`, `host_script_runs`, and `host_script_outputs`.
- `src/Hashi.Infrastructure/Persistence/Entities/PlatformEntities.cs:3-48` lacks resource ownership, domain mode, sync state, last applied hash, and owning workflow fields.
- `src/Hashi.Infrastructure/Persistence/Entities/ExtendedPlatformEntities.cs:38-80` puts many firewall host details in one table without separate rule ownership/history.
- `src/Hashi.Infrastructure/Persistence/Entities/ExtendedPlatformEntities.cs:380-412` collapses notification providers and scripts without delivery/run/output history.

## Expected outcome

The schema should contain enough relational state to enforce ownership and required minimums, expose background job status/diff/error details, record privileged script runs and outputs, track notifications, and reconcile external providers safely.

## Fix guidance

Do not blindly add every table name if an equivalent model is demonstrably better, but add the missing state required by behavior. Start with ownership/run-history tables that unblock critical safety fixes: DNS ownership, script targets/runs/outputs, notification secrets/deliveries, firewall generated scripts/rules, and connection health.

## Acceptance criteria

- The model can enforce DNS/resource ownership without provider comments.
- Script runs and outputs are queryable independently of the script definition.
- Notifications have route/delivery history.
- Firewall generated rules/scripts have auditable desired/applied state.
- Background jobs expose last run, next run, status, duration, diff summary, and error details.
