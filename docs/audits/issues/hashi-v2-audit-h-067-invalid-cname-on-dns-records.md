# H-067: Invalid CNAME Records Pointing to IP Addresses

**Priority:** High
**Conflict Type:** wrong_implementation
**Spec Reference:** Main Spec §14.2

**Status:** In Progress
**Branch:** h/monitoring-dns-firewall
**Branch:** 

## Description

When `LinkedTraefikHost` is unset and `InternalTraefikIp` is set (the normal case for a firewall host), `ResolveOnRouteTarget` returns the IP address. This is passed to `GenerateHostRecords` which creates `on.machine1 CNAME 10.0.0.1` — CNAME records cannot point to IP addresses per DNS spec (RFC 1034). This produces invalid DNS records that will be rejected by DNS providers. The `on.CNAME` should always default to `via` (the CNAME to the A record) unless a hostname override is provided.

## Evidence

- `DnsDesiredStateBuilder.ResolveOnRouteTarget()` returns `InternalTraefikIp` when `LinkedTraefikHost` is empty (`DnsDesiredStateBuilder.cs:197-200`)
- `GenerateHostRecords` uses `OnRouteTarget` as the CNAME target regardless of whether it's an IP or hostname

## Expected Outcome

`on.machine1.example.com` always resolves to a valid CNAME target (a hostname, not an IP). Default should be `via.machine1.example.com`. Only a hostname override should change the CNAME target.

## Fix Guidance

1. `ResolveOnRouteTarget` should return null/empty when only an IP is available.
2. `GenerateHostRecords` should fall back to `via.{host}.{domain}` when `OnRouteTarget` is empty or looks like an IP address.

## Acceptance Criteria

- [ ] `on.machine1.example.com` CNAME always points to a hostname, never an IP
- [ ] Default CNAME target is `via.machine1.example.com`
- [ ] Explicit hostname overrides work correctly
