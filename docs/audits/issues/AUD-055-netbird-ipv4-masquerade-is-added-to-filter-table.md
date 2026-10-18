# AUD-055: NetBird IPv4 routing-peer masquerade is added to the wrong table

Priority: P1

Category: Routing correctness / NetBird

Spec reference: Base §§14.3,14.4.

## Finding

When NetBird routing-peer mode has IPv4 routed CIDRs and an IPv4 overlay, RenderNetBirdRules emits iptables -A HASHI_POSTROUTING ... -j MASQUERADE without -t nat. MASQUERADE belongs in nat POSTROUTING. This causes a command failure under set -e rather than configuring the advertised routing-peer behavior. The IPv6 counterpart does select nat.

## Evidence

- src/Hashi.Core/Firewall/FirewallScriptRenderer.cs:385-409; IPv4 line396 omits -t nat while IPv6 line408 includes it.

Primary reference: [Official behavior documentation](https://man7.org/linux/man-pages/man8/iptables.8.html).

## Expected outcome

Emit IPv4 masquerade in the nat table and verify complete bidirectional routing-peer behavior.

## Acceptance criteria

- IPv4 routing-peer apply succeeds with valid overlay/routed networks.
- NAT rules are placed only in the intended table/chain.
- Multiple overlay CIDRs and both traffic directions are accounted for.

Verification: confirmed by manual source tracing; no live firewall/provider change was performed.
