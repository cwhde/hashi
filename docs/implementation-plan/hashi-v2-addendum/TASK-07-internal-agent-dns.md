# TASK-07: Internal Agent DNS

## Goal

Generate internal DNS names for Pulse agents through AdGuard Home rewrites under an internal domain such as `hashi.home.arpa`.

This must be DNS-only. It must not create reverse-proxy routing or Traefik resources for `hashi.home.arpa`.

## Spec Context

- Original spec sections: 16, 17, 24, 25.
- Addendum sections: 10, 11.1, 12.6, 13.4, 16.2, 18, 19 Phase F.
- Research references: `RESEARCH-RESOURCES.md` AdGuard and Internal DNS section.

## Current Code Anchors

- AdGuard rewrite flow: `src/Hashi.Infrastructure/Platform/AdGuardSyncService.cs`
- AdGuard rewrite entity/source names: `AdGuardRewriteEntity`, `AdGuardRewriteSourceNames`
- Pulse state: `PulseAgentEntity`
- Pulse service: `PulseAgentService`
- Settings endpoints: `SetupAdvanceEndpoints.cs` `SettingsEndpoints`
- AdGuard UI: `web/src/routes/(admin)/adguard/+page.svelte`
- Pulse UI: `web/src/routes/(admin)/pulse/+page.svelte`
- Settings UI: `web/src/routes/(admin)/settings/+page.svelte`

## Rules

Internal agent DNS:

1. Applies only to Pulse agents.
2. Uses normalized agent name or explicit override.
3. Resolves to selected agent IP by default.
4. Is generated through AdGuard rewrites.
5. Coexists with existing real-domain internal rewrites.
6. Must not replace existing real-domain rewrites.
7. Must not create Traefik routers for `hashi.home.arpa`.
8. Must never touch manual AdGuard rewrites not owned by Hashi.

## Name Normalization

Implement and test:

- Lowercase.
- ASCII slug.
- Replace spaces/invalid chars with hyphens.
- Collapse repeated hyphens.
- Trim leading/trailing hyphens.
- Reject empty result.
- Detect collisions and require manual override.

## Settings

Add:

- Enable internal agent DNS.
- Domain default `hashi.home.arpa`.
- Per-agent DNS enabled.
- Per-agent DNS name override.
- IP mode: selected/private/public, default selected.
- Stale behavior: keep last rewrite by default and mark degraded.

## Sync Integration

Add an AdGuard source name such as `internal_agent_dns`.

Add preview/apply/result flow:

- Compute desired agent rewrites.
- Compare with current AdGuard state.
- Create/update/delete only Hashi-owned `internal_agent_dns` entries.
- Never delete manual rewrites.
- Record sync run, diffs, audit events, and last applied hash.
- Keep last known rewrite for stale agents by default.

## API Deliverables

- `GET /api/pulse/agents/{id}/resolved-targets`
- `GET /api/settings/internal-agent-dns`
- `PUT /api/settings/internal-agent-dns`
- `POST /api/settings/internal-agent-dns/preview-sync`
- `POST /api/settings/internal-agent-dns/apply-sync`

## Frontend Deliverables

- Setup optional step for internal agent DNS:
  - enable flag
  - domain
  - warning that it is DNS-only
  - AdGuard requirement
- Settings panel for domain and stale behavior.
- Pulse per-agent DNS name/IP mode controls.
- AdGuard page marks internal-agent rewrites distinctly.

## Tests

- Name normalization and collision handling.
- Desired rewrite generation.
- Stale agent keeps last rewrite by default.
- Existing real-domain topology rewrites still work.
- No Traefik resources/routers for `hashi.home.arpa`.
- Manual AdGuard rewrites remain untouched.
- Preview/apply/result and audit events.

## Acceptance

- `agent.hashi.home.arpa` resolves to selected agent IP through AdGuard.
- Existing real-domain rewrites still work.
- Internal agent DNS never becomes a reverse-proxy routing layer.
