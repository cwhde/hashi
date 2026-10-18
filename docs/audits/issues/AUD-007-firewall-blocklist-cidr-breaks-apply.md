# AUD-007: Firewall apply fails when an IP blocklist contains a CIDR

Priority: P1 — operational security / blocking

Spec reference: implementation spec §13.4 and §14.3 require blocked IPs to be enforced by the generated firewall.

## Finding

`FirewallApplyService` passes enabled IP and CIDR blocklist values into the firewall definition. `FirewallScriptRenderer` creates `hashi_blocked` and `hashi_blocked6` as `hash:ip` sets, then emits every blocked value as an `ipset add`. `hash:ip` accepts a single address, not a network prefix. A CIDR such as `192.0.2.0/24` or `2001:db8::/32` therefore makes `ipset add` fail. Since the generated script uses `set -euo pipefail`, apply aborts at that value and can leave a partially updated firewall.

## Evidence

- `src/Hashi.Infrastructure/Platform/FirewallApplyService.cs` selects enabled `Ip` and `Cidr` blocklist entries for the firewall definition.
- `src/Hashi.Core/Firewall/FirewallScriptRenderer.cs` emits blocked values to the blocked sets and declares both sets as `hash:ip`.
- The same renderer begins the script with `set -euo pipefail`, so an invalid set insertion terminates execution.

## Expected outcome

The firewall must successfully apply individual IP and CIDR block entries, with IPv4 and IPv6 network values enforced by compatible set types. A failed apply must restore the prior firewall state.

## Acceptance criteria

- IPv4 and IPv6 host addresses and CIDRs are accepted by the generated blocked sets.
- A mixed blocklist applies fully and remains effective after restart.
- An invalid entry fails validation before any live rules are replaced, or triggers reliable rollback.
