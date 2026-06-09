# H-078: No Guard Against Traefik Routers for home.arpa Domains

**Priority:** High
**Conflict Type:** missing_implementation
**Spec Reference:** Addendum §10.2

## Description

There is no explicit guard in the Traefik rendering/sync code that prevents creating routers for `hashi.home.arpa` domains. Internal agent DNS only creates AdGuard rewrites, not Traefik routers. But if a resource were accidentally configured with a `hashi.home.arpa` domain, the Traefik sync would create a router for it without any warning or rejection. The spec requires an explicit guard against this.

## Evidence

- No validation in `TraefikConfigRenderer` or `TraefikSyncService` rejects domains ending in `.home.arpa` or matching the internal agent DNS domain
- `ResourceEntity.Domain` has no validation against internal DNS domains

## Expected Outcome

Resources cannot be configured with `hashi.home.arpa` domains. If attempted, the system rejects the domain with a clear error message explaining that this domain is reserved for internal agent DNS only.

## Fix Guidance

1. Add domain validation in resource create/update that rejects domains matching the configured internal agent DNS domain (default: `hashi.home.arpa`) or any `.home.arpa` subdomain.
2. Add the same check in `TraefikConfigRenderer` as a safety net.

## Acceptance Criteria

- [ ] Creating a resource with `hashi.home.arpa` domain returns a validation error
- [ ] Traefik renderer skips any resource with an internal DNS domain
- [ ] Error message clearly explains the domain reservation
