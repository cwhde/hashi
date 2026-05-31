# B01 - Setup creates connection types the backend does not use

Priority: Critical

Spec conflicts: non-negotiable rules 7, 13, and 21; sections 7.5, 7.6, 7.7, 10, 14, and 27. Setup must leave one DNS provider, one Traefik connection, and one Linux firewall host that participate in DNS, Traefik, and firewall sync.

## Problem

The setup and admin connection UIs submit `traefik` and `firewall` as connection types, but the backend canonical names are `traefik_host` and `firewall_host`. The generic SSH endpoint accepts the UI value unchanged, and the validator only requires the field to be non-empty.

That means a connection created by the setup flow is not recognized as the required Traefik connection and is not selected by Traefik sync, access-log ingest, or script-target validation. The firewall host entity can still be created, but its backing SSH connection has the wrong type for firewall-host-only workflows.

## Evidence

- `src/Hashi.Infrastructure/Persistence/Entities/DnsEntities.cs:160-161` defines `TraefikHost = "traefik_host"` and `FirewallHost = "firewall_host"`.
- `src/Hashi.Infrastructure/Auth/SetupCompletionService.cs:53` requires an enabled `ConnectionTypeNames.TraefikHost`.
- `src/Hashi.Infrastructure/Sync/SyncOrchestratorService.cs:268` selects Traefik sync connections using `ConnectionTypeNames.TraefikHost`.
- `src/Hashi.Infrastructure/Platform/AccessLogIngestWorker.cs:31` reads access logs only from `ConnectionTypeNames.TraefikHost`.
- `web/src/lib/components/setup/steps/TraefikConnectionStep.svelte:35` sends `connectionType: 'traefik'`.
- `web/src/lib/components/setup/steps/FirewallHostStep.svelte:32` sends `connectionType: 'firewall'`.
- `web/src/routes/(admin)/connections/+page.svelte:26` defaults the manual connection form to `connectionType: 'traefik'`.
- `src/Hashi.Core/Validation/RequestValidators.cs:25-27` only checks that `ConnectionType` is present.
- `src/Hashi.Api/Features/Connections/ConnectionEndpoints.cs:43-45` passes the raw request value into `SshConnectionService.CreateAsync`.

## Expected outcome

Setup-created Traefik and firewall connections must be stored with the same canonical types consumed by setup completion, sync orchestration, access-log ingest, firewall workflows, and script targeting.

## Fix guidance

Use one shared contract for connection type values across frontend and backend. Either expose the backend constants through the API or make the request use enum-like values that are mapped server-side. Reject unknown connection types. Add regression tests that create connections through the setup payloads and verify setup completion and sync selection see them.

## Acceptance criteria

- Setup Traefik connection creation stores `traefik_host`.
- Setup firewall SSH connection creation stores `firewall_host`.
- Unknown connection types are rejected by validation.
- Setup completion recognizes connections created through the setup UI.
- Traefik sync and access-log ingest select setup-created Traefik hosts.
