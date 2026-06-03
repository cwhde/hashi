# E03 - DNS target matching drops private manual and routed host targets

Priority: High

Spec conflicts: section 15.4 requires Hashi to match manual or Pulse IPs against managed Linux firewall hosts, managed subnets, NetBird-routed subnets, and configured host FQDNs before falling back to public A/AAAA records.

## Problem

Resource DNS generation only passes firewall host public IP and managed subnets into the DNS matcher. It drops private manual resource target IPs before matching, so a manual target inside a managed subnet cannot produce the required `CNAME resource.example.com -> on.host.example.com`. It also does not feed NetBird-routed CIDRs or configured host FQDN/on-route targets into DNS matching.

This leaves several spec-required cases uncovered:

- Manual private IP in a managed subnet.
- Manual or Pulse IP in a NetBird-routed subnet.
- Manual or Pulse target that maps through a configured host FQDN.

## Evidence

- `docs/implementation-spec/hashi-v2-implementation-spec.md:857-859` says the UI requires a manual target IP or Pulse target, then Hashi first matches that IP against managed hosts, managed subnets, NetBird-routed subnets, or configured host FQDN before creating A/AAAA records.
- `docs/implementation-spec/hashi-v2-implementation-spec.md:861-865` says Pulse internal and public IPs are both evaluated against system-managed hosts, and matching hosts produce CNAMEs to `on.host.example.com`.
- `docs/implementation-spec/hashi-v2-implementation-spec.md:1859-1860` lists Pulse DNS sync and managed-host CNAME generation as exit criteria.
- `src/Hashi.Infrastructure/Dns/DnsDesiredStateBuilder.cs:41-48` builds `FirewallHostDnsTarget` from host ID, name, public IP, `null` on-route target, and `ManagedSubnetsJson` only.
- `src/Hashi.Infrastructure/Dns/DnsDesiredStateBuilder.cs:78-86` passes `ResolveManualPublicIp(resource.TargetHost)` as the manual IP candidate.
- `src/Hashi.Infrastructure/Dns/DnsDesiredStateBuilder.cs:165-180` returns `null` for private IPv4, loopback, and link-local/site-local manual target IPs before the matcher sees them.
- `src/Hashi.Core/Dns/DnsRecordGenerator.cs:6-11` has no field for NetBird-routed CIDRs or configured host FQDNs on `FirewallHostDnsTarget`.
- `src/Hashi.Core/Dns/DnsRecordGenerator.cs:92-114` matches candidates only against host public IP and `ManagedSubnets`.

## Expected outcome

DNS desired-state generation should evaluate all manual and Pulse address candidates against the full managed-host topology before deciding whether to emit CNAME, A, or AAAA records.

## Fix guidance

Keep the raw manual IP candidate, including private addresses, for host matching. Extend the DNS host target model with NetBird-routed CIDRs and any configured host FQDN/on-route target that the spec expects. Validate or clearly reject private manual targets that do not match any managed host rather than silently returning no record.

## Acceptance criteria

- A resource with manual target `10.0.0.25` and a firewall host managing `10.0.0.0/24` generates a CNAME to `on.<host>.<root>`.
- A manual or Pulse IP in a firewall host's NetBird-routed CIDR generates the same managed-host CNAME.
- A manual or Pulse target that maps through a configured host FQDN generates the managed-host CNAME.
- An unmatched public manual IP still generates the correct A or AAAA record.
- An unmatched private manual target fails validation or surfaces a clear DNS-plan warning instead of silently producing no generated resource DNS record.
