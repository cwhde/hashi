# E06 - Resource model omits proxy protocol and monitoring hints

Priority: Medium

Spec conflicts: section 6 lists proxy protocol option for TCP resources and monitoring protocol hint as part of the resource model.

## Problem

The current resource model does not persist, expose, or render the TCP proxy protocol option, and it does not provide a per-resource monitoring protocol hint. Monitoring is derived from the resource kind and target scheme only.

This means operators cannot configure proxy protocol for TCP resources through Hashi, and they cannot override automatic monitor type selection for resources whose public behavior differs from the backend target kind or scheme.

## Evidence

- `docs/implementation-spec/hashi-v2-implementation-spec.md:195-198` lists target host, target port, proxy protocol option for TCP, and monitoring protocol hint in the resource model.
- `src/Hashi.Infrastructure/Persistence/Entities/PlatformEntities.cs:3-66` defines `ResourceEntity` without proxy protocol or monitoring hint fields.
- `src/Hashi.Contracts/Api/PlatformContracts.cs:3-29`, `:71-92`, and `:94-124` define resource response, create request, and update request without those fields.
- `src/Hashi.Core/Resources/ResourceModels.cs:67-88` defines `ResourceDefinition` without those fields, so the Traefik renderer cannot receive them.
- `src/Hashi.Core/Traefik/TraefikConfigRenderer.cs:417-431` renders TCP routers/services without proxy protocol configuration.
- `src/Hashi.Infrastructure/Platform/MonitoringService.cs:426-456` derives monitor check type and URL from resource kind, target scheme, and target host/port.

## Expected outcome

Resource configuration should include the TCP proxy protocol option and a monitoring protocol hint, and those values should flow through persistence, API contracts, UI, renderers, and monitoring provisioning.

## Fix guidance

Add nullable fields for proxy protocol and monitoring hint to the resource persistence model and API contracts. Normalize and validate allowed monitoring hints against the existing monitor check types. Feed proxy protocol into Traefik TCP rendering and feed monitoring hints into `MonitoringService` when provisioning resource-owned monitor endpoints.

## Acceptance criteria

- TCP resources can enable or disable proxy protocol through the API and UI.
- Rendered TCP Traefik config reflects the chosen proxy protocol behavior.
- Resources can set a monitoring hint such as HTTP, HTTPS, H2C, TCP, UDP, DNS, TLS, ICMP, or Pulse where appropriate.
- Resource-owned monitor endpoint provisioning uses the monitoring hint when present and falls back to current inference when absent.
- Tests cover proxy protocol rendering and monitoring-hint provisioning.
