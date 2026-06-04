# TASK-06: Agent-Bound Connections

## Goal

Allow connection-like integrations to target a Pulse agent's resolved IP instead of only a static host/IP. The first complete path should be AdGuard Home; the model should be reusable for Traefik/firewall/future service connections.

## Spec Context

- Original spec sections: 6, 15, 16, 17, 24, 25.
- Addendum sections: 9, 12.6, 16.2, 18, 19 Phase E.

## Current Code Anchors

- Pulse data/service: `PulseAgentEntity`, `PulseAgentService`
- Connection entities/service: `ConnectionEntity`, `SshConnectionService`, `ConnectionEndpoints.cs`
- AdGuard entity/service: `AdGuardConnectionEntity`, `AdGuardSyncService`
- Current connection contracts: `ConnectionContracts.cs`, `PlatformContracts.cs`
- Connections UI: `web/src/routes/(admin)/connections/+page.svelte`
- AdGuard UI: `web/src/routes/(admin)/adguard/+page.svelte`
- Pulse UI: `web/src/routes/(admin)/pulse/+page.svelte`

## Target Model

Support fields:

- `target_mode`: `static_host`, `static_ip`, `pulse_agent`
- `static_host`
- `static_ip`
- `pulse_agent_id`
- `pulse_ip_mode`: `selected`, `public`, `private_selected`, `private_candidate`
- `private_candidate_selector`
- `port`
- `scheme`
- `path_prefix`
- `tls_validation_mode`
- `expected_hostname`
- `resolved_ip_snapshot`
- `last_resolved_at_utc`
- `resolution_status`
- `resolution_error`

Default UI mode should be `selected`.

## Resolver Service

Add a reusable resolver service, for example `ConnectionTargetResolver`, that:

- Resolves static host/IP targets.
- Resolves Pulse agent selected/public/private IP modes.
- Validates stale or missing heartbeat state.
- Captures resolved snapshot and error.
- Provides connectivity test hooks without leaking secrets.
- Produces dependency impact records when resolved target changes.

## AdGuard Integration

Replace or wrap `AdGuardConnectionEntity.BaseUrl`:

- Existing static URL connections must keep working.
- New Pulse target mode builds the base URI from scheme, resolved IP, port, and path prefix.
- `CreateAuthorizedClientAsync` should use the resolver.
- Health/test calls should show resolved target, status, and stale state.

## Pulse IP Change Behavior

When a Pulse agent changes resolved IP:

1. Record new resolved target.
2. Mark dependent connection health pending.
3. Queue relevant sync/reconcile jobs.
4. Do not rewrite unrelated resources.
5. Record dependency impact in activity/audit logs.

## Frontend Deliverables

- Connection/AdGuard target mode selector:
  - static host
  - static IP
  - Pulse agent
- Pulse agent picker with last seen, public IP, private candidates, selected IP, stale status, reachability.
- Explicit confirmation to save inactive/unreachable targets.
- Resolved target display on connection detail/list.

## Tests

- Agent must exist.
- Valid/invalid Pulse IP modes.
- Selected private candidate by interface/CIDR/address.
- Stale agent target behavior.
- AdGuard target resolves from Pulse agent.
- Static target compatibility.
- Agent-bound target changes create audit events.

## Acceptance

- AdGuard can be configured via a Pulse agent target.
- Static AdGuard connections still work.
- Stale/unreachable agent target is visible and safe.
- Model can be reused by future Traefik/firewall connection work.
