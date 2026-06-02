# C02 - Hashi internal URLs are still hard-coded to port 8080

Priority: High

Spec conflicts: deployment ports are configurable with admin default `8080`, public dashboard default `8081`, and public status default `8082`; section 10.3 also requires generated Traefik dynamic files to be valid for the configured Hashi deployment.

## Problem

Most port configuration was added, but several generated internal URLs still assume the admin API listens on `127.0.0.1:8080`. If the operator changes the admin port, Traefik forward-auth, Traefik health routing, and the built-in Hashi API monitor can point at the wrong port.

This is worse than a cosmetic default: the spec makes the admin port configurable, and these hard-coded references sit in runtime-generated routing and monitoring data.

## Evidence

- `docs/implementation-spec/hashi-v2-implementation-spec.md:88-92` defines the admin, dashboard, and status ports as configurable defaults.
- `src/Hashi.Api/Hosting/HashiPorts.cs:10-18` exposes configurable admin/dashboard/status port options.
- `src/Hashi.Infrastructure/Persistence/Entities/CoreEntities.cs:11` has an `InternalUrl` setting that could represent the Hashi internal base URL.
- `src/Hashi.Core/Traefik/TraefikConfigRenderer.cs:27` defaults `HashiForwardAuthUrl` to `http://127.0.0.1:8080/api/edge-auth/forward`.
- `src/Hashi.Core/Traefik/TraefikConfigRenderer.cs:155-167` renders forward-auth middleware using that URL.
- `src/Hashi.Core/Traefik/TraefikConfigRenderer.cs:495-509` renders the Hashi health service as `http://127.0.0.1:8080/api/health`.
- `src/Hashi.Infrastructure/Platform/MonitoringService.cs:289-295` provisions the Hashi API monitor with `http://127.0.0.1:8080/api/health`.

## Expected outcome

Generated internal Hashi URLs should derive from the configured admin/internal URL, not the default port.

## Fix guidance

Introduce one internal base URL resolver that uses `AppSettingsEntity.InternalUrl` when set, otherwise derives from `HashiPortOptions.Admin`. Pass the resolved URLs into `TraefikRenderOptions` and monitoring endpoint provisioning. Add tests that set a non-8080 admin port/internal URL and assert no generated config still contains `127.0.0.1:8080`.

## Acceptance criteria

- Forward-auth middleware points at the configured Hashi internal URL.
- The generated Traefik health service points at the configured Hashi internal URL.
- The built-in Hashi API monitor points at the configured Hashi internal URL.
- Tests cover a non-default admin port and fail on lingering `:8080` references in generated config.
