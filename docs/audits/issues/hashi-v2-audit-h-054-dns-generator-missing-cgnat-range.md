# H-054: DnsRecordGenerator IsPrivateIpv4 Misses CGNAT Range 100.64.0.0/10

**Priority:** Low
**Conflict Type:** bad_implementation
**Spec Reference:** Main Spec §15.4, §17.4

**Status:** Fixed
**Branch:** h/backend-quality

## Description

`DnsRecordGenerator.IsPrivateIpv4` in `src/Hashi.Core/Dns/DnsRecordGenerator.cs` identifies private IPv4 addresses but does not include the Carrier-Grade NAT (CGNAT) range `100.64.0.0/10` (RFC 6598):

The standard private IPv4 ranges are:
- `10.0.0.0/8` ✓ (RFC 1918)
- `172.16.0.0/12` ✓ (RFC 1918)
- `192.168.0.0/16` ✓ (RFC 1918)
- `100.64.0.0/10` ✗ (RFC 6598 — CGNAT/Shared Address Space)

In homelab deployments that use CGNAT ranges (e.g., behind certain ISP routers or in container networking), IPs in `100.64.0.0/10` should be treated as private/internal and not used for public DNS A records.

The spec §15.4 and §17.4 both require matching IPs to managed hosts before creating A records. Missing the CGNAT range means CGNAT IPs could be treated as "public" and published in DNS.

## Evidence

```csharp
// DnsRecordGenerator.cs — IsPrivateIpv4 likely checks only:
// 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16
// Missing: 100.64.0.0/10
```

The Pulse agent's `isPrivateIPv6` covers ULA `fc00::/7` correctly. But the IPv4 check in `DnsRecordGenerator` doesn't mirror the completeness.

## Expected Outcome

`IsPrivateIpv4` should include `100.64.0.0/10` in its check. All standard private/restricted IPv4 ranges should be covered:
- `10.0.0.0/8`
- `172.16.0.0/12`
- `192.168.0.0/16`
- `100.64.0.0/10` (CGNAT)
- `127.0.0.0/8` (loopback)
- `169.254.0.0/16` (link-local)

## Fix Guidance

1. Add `100.64.0.0/10` to the private range check.
2. Consider adding `169.254.0.0/16` (link-local) as well.
3. Add unit tests verifying all ranges are correctly identified.

## Acceptance Criteria

- [ ] `100.64.0.0/10` is treated as private/internal
- [ ] `169.254.0.0/16` is treated as private/internal (or explicitly documented as not)
- [ ] Unit tests cover all private range boundaries
- [ ] CGNAT IPs from Pulse agents are not published as public DNS records
