# B04 - Public and admin ports are hard-coded

Priority: Medium

Spec conflicts: section 4 says the public dashboard, public status, and admin ports are configurable with defaults of 8081, 8082, and 8080.

## Problem

The API binds directly to 8080, 8081, and 8082 in code, and the frontend public-port router also keys off those literal browser ports. The compose and Docker config expose the same fixed ports, but there is no application setting or environment variable path that changes the runtime port map coherently.

This prevents users from deploying Hashi on alternate host ports while preserving the three-port behavior the spec describes.

## Evidence

- `src/Hashi.Api/Program.cs:26-28` calls `ListenAnyIP(8080)`, `ListenAnyIP(8081)`, and `ListenAnyIP(8082)` directly.
- `src/Hashi.Api/Hosting/HashiPorts.cs:5-7` defines the same ports as constants.
- `src/Hashi.Api/Program.cs:74` uses the constants to decide public origins.
- `web/src/lib/public/port-mode.ts:10-14` treats only browser ports `8081` and `8082` as public roots.
- `deploy/compose/docker-compose.yml:24-28` maps and configures only the fixed default ports.
- `deploy/docker/Dockerfile:27-28` sets `ASPNETCORE_URLS` to 8080 and exposes 8080/8081/8082, but `Program.cs` overrides Kestrel listeners.

## Expected outcome

The default ports should remain 8080, 8081, and 8082, but operators must be able to configure the admin, dashboard, and status ports through appsettings or environment variables. Server routing and frontend root-mode detection must use the same configured values.

## Fix guidance

Introduce a typed port configuration object with defaults. Bind Kestrel from that configuration, update `HashiPorts` consumers to use options instead of constants, and expose the configured public modes to the frontend through server-rendered config or an API-safe bootstrap value.

## Acceptance criteria

- Changing configured admin/dashboard/status ports changes Kestrel listeners.
- Public-port routing honors the configured dashboard and status ports.
- The frontend root page resolves dashboard/status modes for configured public ports.
- Docker and compose examples document the default values and how to override them.
