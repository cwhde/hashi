# B10 - Monitoring provisioning does not cover required sources or check types

Priority: High

Spec conflicts: section 18 and Phase 8. Monitoring must support resources, manual DNS entries, firewall hosts, Traefik connections, AdGuard, Hashi itself, user-created endpoints, and HTTP, HTTPS, H2C, TCP, UDP, DNS, ICMP, TLS cert, and Pulse checks.

## Problem

The monitor worker has implementations for many check types, but endpoint provisioning only creates monitor endpoints from resources with `StatusEnabled` and a domain. It maps `http` resources to HTTP checks and every other resource kind to HTTPS checks. TCP, UDP, H2C, Pulse, DNS, ICMP, and TLS checks are therefore not provisioned correctly from the relevant objects.

The status API is read-only, so users cannot create the manual/user-defined monitor endpoints required by the spec.

## Evidence

- `src/Hashi.Infrastructure/Platform/MonitoringService.cs:11-20` syncs monitor endpoints only from enabled resources with `StatusEnabled` and `Domain`.
- `src/Hashi.Infrastructure/Platform/MonitoringService.cs:20` maps all non-`http` resource kinds to `https`.
- `src/Hashi.Api/Features/Platform/PlatformEndpoints.cs:242-266` exposes only `GET /api/status/endpoints`, `GET /api/status/rollups`, and `GET /api/status/events`.
- `src/Hashi.Infrastructure/Platform/MonitorCheckWorker.cs:148-216` can execute HTTP, HTTPS, H2C, TCP, UDP, DNS, ICMP, TLS, and Pulse checks, but those endpoint types are not created by provisioning.
- `src/Hashi.Infrastructure/Platform/MonitoringService.cs:123-128` has special handling for Pulse-linked monitor endpoints, but `SyncEndpointsFromResourcesAsync` never creates a `pulse` check.

## Expected outcome

Hashi should provision or let users create monitor endpoints for all source types listed in the spec, and each endpoint should use an appropriate check type.

## Fix guidance

Add monitor endpoint CRUD or a manual endpoint management API. Extend automatic provisioning for resource kinds, manual DNS entries with monitoring enabled, firewall hosts, Traefik connections, AdGuard connections, Hashi internal health, and Pulse agents. Map `tcp` resources to TCP checks, `udp` resources to UDP checks, H2C resources to H2C checks, and Pulse-linked resources to Pulse checks where intended.

## Acceptance criteria

- Users can create, update, disable, and delete manual monitor endpoints.
- TCP and UDP resources create TCP and UDP checks rather than HTTPS checks.
- Firewall host, Traefik, AdGuard, Hashi, manual DNS, and Pulse sources appear as monitor endpoints where enabled.
- Tests cover endpoint provisioning and check-type mapping for every spec-listed check type.
