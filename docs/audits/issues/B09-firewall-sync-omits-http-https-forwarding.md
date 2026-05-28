# B09 - Firewall sync omits HTTP and HTTPS forwarding

Priority: High

Spec conflicts: sections 7.7, 10, 14, and 27. Firewall hosts must DNAT configured public ports to the linked internal Traefik IP, including the standard HTTP and HTTPS edge ports used by normal resources and the Hashi system resource.

## Problem

Firewall host definitions build DNAT port forwards only from enabled `tcp` and `udp` resources. HTTP and HTTPS resources are excluded, even though Traefik treats ports 80 and 443 as confirmed edge entry points by default.

As a result, a normal HTTP/HTTPS resource or the Hashi system resource can have DNS and Traefik config, while the generated firewall script does not forward 80/443 to the internal Traefik target.

## Evidence

- `src/Hashi.Infrastructure/Platform/FirewallApplyService.cs:78-80` selects only resources where `x.Kind == "tcp" || x.Kind == "udp"`.
- `src/Hashi.Infrastructure/Platform/FirewallApplyService.cs:81-87` creates port forwards only from that stream-resource list.
- `src/Hashi.Infrastructure/Platform/FirewallApplyService.cs:108` passes that limited set into `FirewallHostDefinition`.
- `src/Hashi.Infrastructure/Platform/TraefikEntryPointService.cs:74-82` treats ports 80/tcp and 443/tcp as confirmed.
- `src/Hashi.Infrastructure/Services/SystemResourceSetupService.cs:61-67` creates the Hashi system resource as `Kind = "https"`, so it is not a `tcp` or `udp` stream resource.

## Expected outcome

Firewall sync must include HTTP and HTTPS forwarding for the linked Traefik host when resources need public web entry points, including the Hashi system resource.

## Fix guidance

Add standard web port forwards for enabled HTTP/HTTPS resources assigned to a firewall host or for any host serving the system resource. De-duplicate forwards by port/protocol and keep the confirmation model for non-standard public ports. Include preview output so opening 80/443 remains visible before apply.

## Acceptance criteria

- A firewall host serving an HTTPS resource generates a 443/tcp DNAT to the linked internal Traefik IP.
- A firewall host serving an HTTP resource generates an 80/tcp DNAT when appropriate.
- The Hashi system resource causes the needed web forwards to appear in the firewall plan.
- TCP/UDP stream forwarding still works and remains de-duplicated.
- Tests cover firewall definitions for HTTPS, HTTP, TCP, UDP, and system-resource cases.
