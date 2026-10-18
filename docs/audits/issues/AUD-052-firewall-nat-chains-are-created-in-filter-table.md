# AUD-052: Firewall NAT chains are created in the filter table

Priority: P1

Category: Installability / firewall forwarding

Spec reference: Base §§14.3,14.5.

## Finding

On a fresh host, the script creates HASHI_DNAT and HASHI_POSTROUTING with iptables -N and ip6tables -N without -t nat. Those chains therefore exist in the filter table. It subsequently inserts jumps from nat PREROUTING/POSTROUTING to chains absent from the nat table. Under set -e, apply aborts. If the host already has nat chains from an earlier deployment, the script does not flush those nat chains and appends new rules instead, so stale forwarding can survive. FirewallApplyService preflight does not prepare nat chains.

## Evidence

- src/Hashi.Core/Firewall/FirewallScriptRenderer.cs:242-262 creates chains without table selection and then references them in nat.
- src/Hashi.Infrastructure/Platform/FirewallApplyService.cs:668-695 checks command availability only.

Primary reference: [Official behavior documentation](https://man7.org/linux/man-pages/man8/iptables.8.html).

## Expected outcome

Create and flush Hashi NAT chains in the nat table for both address families, keeping filter chains in filter.

## Acceptance criteria

- Fresh-host application creates all required chains in the intended tables.
- Repeated application removes stale managed NAT rules and introduces no duplicates.
- Only Hashi chains are changed; unrelated firewall state is preserved.

Verification: confirmed by manual source tracing; no live firewall/provider change was performed.
