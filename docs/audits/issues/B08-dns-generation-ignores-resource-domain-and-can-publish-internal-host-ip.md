# B08 - DNS generation ignores resource domain and can publish internal host IP

Priority: High

Spec conflicts: sections 6, 15, 16, and Phase 12. Resource DNS must honor the configured resource domain mode and public host records must use public/manual/Pulse targets rather than leaking internal addresses.

## Problem

The DNS desired-state builder filters resources by `resource.Domain != null`, but it does not pass that domain into DNS generation. The generator always emits records for `slug.rootDomain`, while Traefik routes use `resource.Domain`. Custom domains, full-domain resources, and root-domain resources therefore get Traefik config for one host and DNS for another.

The same builder also creates firewall-host DNS targets with `h.PublicIp ?? h.InternalTraefikIp`. If a firewall host has no public IP, Hashi can publish the internal Traefik IP as the public A record for the host.

## Evidence

- `src/Hashi.Infrastructure/Persistence/Entities/PlatformEntities.cs:9` stores a resource slug and `src/Hashi.Infrastructure/Persistence/Entities/PlatformEntities.cs:29` stores the resource domain separately.
- `src/Hashi.Core/Traefik/TraefikConfigRenderer.cs:250` builds the Traefik rule from `resource.Domain`.
- `src/Hashi.Infrastructure/Dns/DnsDesiredStateBuilder.cs:52-54` selects resources with a non-empty domain.
- `src/Hashi.Infrastructure/Dns/DnsDesiredStateBuilder.cs:65-72` creates `ResourceDnsTarget` from `resource.Name`, `slug`, and `rootDomain`, but not `resource.Domain`.
- `src/Hashi.Core/Dns/DnsRecordGenerator.cs:53` computes `resourceFqdn` as `{target.Slug}.{target.RootDomain}`.
- `src/Hashi.Infrastructure/Dns/DnsDesiredStateBuilder.cs:38-44` sets firewall-host public DNS target IP to `h.PublicIp ?? h.InternalTraefikIp`.
- `src/Hashi.Core/Dns/DnsRecordGenerator.cs:42` emits the host A record from `host.PublicIp`.

## Expected outcome

DNS records for resources must match the actual public hostnames Hashi configures in Traefik. Firewall host records must only publish a valid public/manual/Pulse target, never an internal Traefik IP as a public host A record.

## Fix guidance

Represent resource DNS intent explicitly: root-domain, subdomain, and custom/full-domain modes should resolve to the exact FQDN Traefik will serve. Use the resource domain in DNS generation, not only slug plus root domain. For firewall hosts, require a public IP, matching Pulse/manual target, or skip the public host A record with a clear warning.

## Acceptance criteria

- A custom-domain resource produces DNS for that custom domain.
- A root-domain resource produces DNS for the root domain when selected.
- Traefik host rules and DNS desired records use the same FQDN.
- Firewall-host DNS generation never publishes `InternalTraefikIp` as the public host A record.
- Tests cover slug subdomain, root domain, custom domain, Pulse target, and no-public-IP firewall host cases.
