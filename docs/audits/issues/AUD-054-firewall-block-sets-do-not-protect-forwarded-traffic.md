# AUD-054: Firewall block sets do not protect forwarded service traffic

Priority: P1

Category: Security / firewall enforcement

Spec reference: Base §13.4; extended addendum §§7.2,15.

## Finding

The generated script tests hashi_blocked and hashi_blocked6 only in HASHI_INPUT. Packets DNATed to the internal Traefik host traverse FORWARD, where HASHI_FWD has accepts for configured ports and managed subnets but no blocked-source drop. A blocked source can still reach forwarded TCP/UDP services, which have no HTTP forward-auth fallback. The public CAPTCHA flow is also intended to remain subject to hard firewall blocks.

## Evidence

- src/Hashi.Core/Firewall/FirewallScriptRenderer.cs:264-272 checks block sets only in INPUT.
- Lines143-148,280-290 create DNAT and FORWARD accepts without a blocked-source check.

Primary reference: [Official behavior documentation](https://man7.org/linux/man-pages/man8/iptables.8.html).

## Expected outcome

Enforce hard source blocks in the forwarded traffic path before NetBird, port-forward, or subnet accepts, for both IP families.

## Acceptance criteria

- A blocked source cannot reach any Hashi forwarded TCP/UDP service.
- Forwarded HTTP/CAPTCHA traffic respects hard blocks independently of application middleware.
- Allowed traffic and established-connection policy have explicit documented precedence.

Verification: confirmed by manual source tracing; no live firewall/provider change was performed.
