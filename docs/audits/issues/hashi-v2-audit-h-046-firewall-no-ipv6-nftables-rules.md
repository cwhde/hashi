# H-046: FirewallScriptRenderer Missing IPv6 Firewall Rules and nftables Option

**Priority:** Medium
**Conflict Type:** missing_implementation
**Spec Reference:** Main Spec §14.3, §14.4, §14.5

**Status:** Fixed
**Branch:** audit-series-h

## Description

`FirewallScriptRenderer` in `src/Hashi.Core/Firewall/FirewallScriptRenderer.cs` generates iptables-only firewall rules with no IPv6 support:

1. **No ip6tables rules**: Only `iptables` (IPv4) commands are generated. IPv6 traffic on managed hosts is not handled.
2. **No nftables support**: The renderer only generates legacy `iptables`/`ipset` rules. Modern Linux distributions (especially those using nftables natively) may not have iptables installed or may have it as a compatibility layer, which can create conflicts.
3. **ipsets are IPv4-only**: The `hashi_trusted`, `hashi_blocked`, and `hashi_netbird` ipsets all use `family inet` (IPv4 only).
4. **Only IPv4 forwarding enabled**: `sysctl -w net.ipv4.ip_forward=1` is set but `net.ipv6.conf.all.forwarding` is never configured.

While the spec §14.3 doesn't explicitly mandate IPv6 support, NetBird overlay networks (§14.4) and modern deployments commonly use IPv6. The spec §14.2 also includes `AAAA` record generation for host DNS, implying IPv6 connectivity should work through the firewall.

## Evidence

```csharp
// FirewallScriptRenderer.cs — all iptables rules, no ip6tables
ipset create hashi_trusted hash:net family inet ...  // IPv4 only
ipset create hashi_blocked hash:ip family inet ...   // IPv4 only
iptables -t nat -A HASHI_DNAT ...                    // IPv4 only
sysctl -w net.ipv4.ip_forward=1                      // IPv4 only
```

## Expected Outcome

1. IPv6 firewall rules should be generated alongside IPv4 rules (ip6tables equivalents for all chains).
2. IPv6 ipsets should be created for trusted and blocked IPv6 addresses.
3. IPv6 forwarding should be enabled (`net.ipv6.conf.all.forwarding=1`).
4. At minimum, the firewall script should document that IPv6 is not managed and warn the user.

## Fix Guidance

1. Generate parallel `ip6tables` rules for all Hashi-owned chains.
2. Add IPv6 ipset equivalents (`family inet6`).
3. Enable IPv6 forwarding.
4. If full IPv6 support is deferred, add a clear comment at the top of the generated script and a warning in the UI.
5. Consider adding an nftables generation option for newer distributions.

## Acceptance Criteria

- [x] IPv6 traffic is handled by Hashi-managed firewall rules
- [x] IPv6 ipsets exist for trusted, blocked, and NetBird addresses
- [x] `net.ipv6.conf.all.forwarding` is configured
- [x] Public ports are DNAT'd for both IPv4 and IPv6
- [ ] Or: clear documentation and UI warning about IPv6 not being managed
