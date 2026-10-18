# AUD-018: DNS subnet matcher accepts out-of-range CIDR prefixes

Priority: P2 — DNS planning correctness / invalid configuration

Category: Edge case bug

Spec reference: implementation spec §§8.1 and 8.3 require safe target matching and DNS desired-state planning.

## Finding

`DnsRecordGenerator.IpMatchesSubnet` parses the prefix as an integer but never checks that it lies between zero and the address width (32 for IPv4, 128 for IPv6). With matching address families, a negative prefix can reach the comparison loop and match arbitrary addresses; a prefix larger than the address width can index beyond the address byte array. The helper is used to associate resource/agent addresses with firewall hosts from persisted `ManagedSubnets` and `NetBirdRoutedCidrs`, so malformed stored CIDR metadata can select the wrong host or throw while constructing DNS desired state.

## Evidence

- `src/Hashi.Core/Dns/DnsRecordGenerator.cs`, `IpMatchesSubnet`: parses `prefixLength`, checks only address-byte lengths, and then indexes using the unchecked prefix.
- The helper is called from `ResolveManagedHost` and `DnsDesiredStateBuilder` while matching configured subnet metadata.

## Expected outcome

Validate prefix bounds against the parsed network address family before comparing bits. Invalid CIDRs should return false or produce a validation error at the configuration boundary; they must not match addresses or abort an unrelated DNS plan.

## Acceptance criteria

- IPv4 prefixes outside 0–32 and IPv6 prefixes outside 0–128 are rejected safely.
- Valid boundary prefixes `/0`, `/32`, and `/128` behave correctly.
- IPv4-mapped IPv6 values have an explicit, consistent address-family policy.
- Invalid subnet values cannot cause DNS planning to throw or match a host.
