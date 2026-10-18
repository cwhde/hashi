# AUD-053: Stream DNAT uses the upstream service port instead of the Traefik entry point

Priority: P1

Category: Routing correctness / TCP and UDP

Spec reference: Base §§10.2,14.3.

## Finding

For a TCP/UDP resource with PublicPort 8443 and TargetPort 3000, Traefik listens on 8443 and forwards to the upstream on 3000. FirewallApplyService instead generates public 8443 -> internalTraefikIp:3000. Traffic bypasses or misses the intended Traefik entry point. This affects every stream resource whose public and upstream ports differ.

## Evidence

- src/Hashi.Infrastructure/Platform/FirewallApplyService.cs:795-802 uses x.TargetPort as the DNAT target port.
- src/Hashi.Core/Traefik/TraefikConfigRenderer.cs:103-110 binds the entry point to EffectivePublicPort.

## Expected outcome

DNAT to the linked Traefik host on its effective public entry-point port, while keeping the actual service port solely in the Traefik upstream definition.

## Acceptance criteria

- Public8443/upstream3000 produces DNAT8443 -> Traefik8443 -> upstream3000.
- TCP and UDP use the same correct distinction.
- Forward allow rules and preview show the same effective chain of destinations.

Verification: confirmed by manual source tracing; no live firewall/provider change was performed.
