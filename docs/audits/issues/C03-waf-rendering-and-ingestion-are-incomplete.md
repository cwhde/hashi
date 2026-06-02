# C03 - WAF rendering and ingestion are incomplete

Priority: High

Spec conflicts: section 12 requires per-resource WAF modes, per-resource exclusions, pinned local Coraza/OWASP CRS middleware, and WAF matches audited into Hashi security events.

## Problem

The security dynamic file is rendered incorrectly when more than one resource has WAF enabled. Each resource renderer returns a complete YAML document with a top-level `http:` key, and `RenderSecurity` concatenates those complete documents. That produces repeated top-level `http:` maps in `40-hashi-security.yml`, so only one middleware may survive YAML parsing or the file may be rejected depending on parser behavior.

The broader WAF requirements are also incomplete: there is no per-resource exclusion model/API, and the WAF event recording method is not wired to any endpoint or log ingestion path.

## Evidence

- `docs/implementation-spec/hashi-v2-implementation-spec.md:571-591` requires per-resource WAF modes, per-resource exclusions, and WAF matches in Hashi security events.
- `src/Hashi.Core/Security/WafModels.cs:21-32` renders a complete `http:` document for one WAF middleware.
- `src/Hashi.Core/Traefik/TraefikConfigRenderer.cs:402-407` concatenates one complete document per WAF-enabled resource.
- `src/Hashi.Core/Traefik/TraefikConfigRenderer.cs:421-423` attaches each router to a per-resource `{slug}-waf` middleware, so missing middlewares break protected resources.
- `tests/Hashi.UnitTests/WafMiddlewareRendererTests.cs:8-15` covers only one WAF middleware.
- `tests/Hashi.UnitTests/PlatformTests.cs:28-45` covers only one WAF-enabled resource in the full renderer.
- `src/Hashi.Infrastructure/Persistence/Entities/PlatformEntities.cs:43-45` has a WAF mode but no WAF exclusion fields.
- `src/Hashi.Infrastructure/Platform/SecurityIngestionService.cs:185-200` defines `RecordWafEventAsync`, but `rg RecordWafEventAsync` finds no caller outside that definition.
- `src/Hashi.Api/Features/Platform/PlatformEndpoints.cs:863-865` only exposes a middleware-rendering GET endpoint, not WAF event ingestion.

## Expected outcome

Multiple WAF-enabled resources should render one valid Traefik dynamic file with all required middlewares. Users should be able to configure per-resource WAF exclusions, and WAF matches should flow into security events.

## Fix guidance

Change WAF rendering so the security file has a single `http.middlewares` map containing all WAF middlewares. Add model/API/UI support for resource-level exclusions and include them in the Coraza directives. Wire WAF logs or plugin callbacks into authenticated/internal ingestion that calls `RecordWafEventAsync`, then cover the path with tests.

## Acceptance criteria

- Rendering two WAF-enabled resources produces exactly one top-level `http:` map and both `{slug}-waf` middleware definitions.
- Traefik config validation covers the multi-resource WAF file.
- Users can define per-resource WAF exclusions.
- WAF events are ingested into `SecurityEvents` without exposing an unauthenticated public write path.
- Tests cover multi-resource rendering, exclusions, and WAF event ingestion.
