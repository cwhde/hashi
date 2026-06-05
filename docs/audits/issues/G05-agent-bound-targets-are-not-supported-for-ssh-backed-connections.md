# G05 - Agent-bound targets are not supported for SSH-backed connections

Priority: Medium

Spec conflicts: addendum sections 9.1, 9.2, 9.5, and 12.6

## Problem

The addendum says connections that currently require a fixed host/IP must also support Pulse agent targets, explicitly including AdGuard Home, Traefik hosts, and Linux firewall hosts where meaningful. The implementation adds a reusable target model and applies it to AdGuard, but Traefik and firewall setup still create plain SSH connections with a static `host` field only.

This leaves the agent-bound connection feature partially implemented. Users can bind AdGuard to a Pulse agent, but they cannot configure the Traefik host or firewall host SSH connection to resolve through a Pulse agent target, and the generic SSH connection validation/write path never resolves connection targets.

## Evidence

- `docs/implementation-spec/hashi-v2-implementation-spec-addendum.md:795` through `docs/implementation-spec/hashi-v2-implementation-spec-addendum.md:804` requires fixed host/IP connections to support Pulse agent targets, including AdGuard, Traefik, and Linux firewall hosts.
- `docs/implementation-spec/hashi-v2-implementation-spec-addendum.md:818` through `docs/implementation-spec/hashi-v2-implementation-spec-addendum.md:839` defines the reusable target model fields.
- `docs/implementation-spec/hashi-v2-implementation-spec-addendum.md:872` through `docs/implementation-spec/hashi-v2-implementation-spec-addendum.md:880` defines validation requirements before saving an agent-bound connection.
- `src/Hashi.Infrastructure/Platform/AdGuardSyncService.cs:685` through `src/Hashi.Infrastructure/Platform/AdGuardSyncService.cs:717` creates a `ConnectionTargetEntity` from AdGuard connection requests, and `web/src/routes/(admin)/adguard/+page.svelte:292` exposes target mode UI for AdGuard.
- `web/src/lib/components/setup/steps/TraefikConnectionStep.svelte:17` through `web/src/lib/components/setup/steps/TraefikConnectionStep.svelte:25` stores Traefik connection inputs as static host/user/password/path fields, and `web/src/lib/components/setup/steps/TraefikConnectionStep.svelte:33` through `web/src/lib/components/setup/steps/TraefikConnectionStep.svelte:44` posts only `host`, `port`, `username`, and credentials.
- `web/src/lib/components/setup/steps/FirewallHostStep.svelte:16` through `web/src/lib/components/setup/steps/FirewallHostStep.svelte:23` stores firewall setup inputs as static host/user/password fields, and `web/src/lib/components/setup/steps/FirewallHostStep.svelte:30` through `web/src/lib/components/setup/steps/FirewallHostStep.svelte:41` posts only a static SSH host.
- `src/Hashi.Infrastructure/Connections/SshConnectionService.cs:27` through `src/Hashi.Infrastructure/Connections/SshConnectionService.cs:39` persists SSH connection settings with only `settings.Host`, `Port`, `Username`, and paths.
- `src/Hashi.Infrastructure/Connections/ConnectionSshCredentialResolver.cs:75` through `src/Hashi.Infrastructure/Connections/ConnectionSshCredentialResolver.cs:85` parses SSH settings directly from the stored host and never resolves a Pulse target.

## Expected outcome

Traefik and firewall SSH-backed connections should support the same target modes as AdGuard where meaningful, including Pulse agent target selection, validation, persisted resolution status, and dependency impact on target changes.

## Fix guidance

Extend the connection API and SSH connection storage to accept `ConnectionTargetRequest` or an equivalent target reference for Traefik/firewall connection types. Resolve the target before SSH validation, writes, and firewall apply. Add UI controls in Traefik and firewall setup/admin screens for static host, static IP, and Pulse agent target modes, following the AdGuard target UI where possible.

## Acceptance criteria

- Traefik host setup can select a Pulse agent target instead of a literal host/IP.
- Firewall host setup can select a Pulse agent target where applicable.
- SSH validation and write/apply paths resolve the current Pulse target before connecting.
- Agent-bound target changes mark dependent Traefik/firewall connection health pending and queue relevant sync work.
- Tests cover SSH-backed target resolution and stale/inactive Pulse-agent validation behavior.
