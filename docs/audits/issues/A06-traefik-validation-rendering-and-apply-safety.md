# A06 - Traefik validation, rendering, and apply safety do not meet spec

Priority: High

Spec conflicts: non-negotiable rules 7, 8, 9, 17, and 20; Traefik manager sections 10 and 24.

## Problem

Traefik validation is mostly substring checks and a handwritten YAML-ish parser. The spec requires safe YAML parsing with a proven library and real Traefik config validation before apply. The apply service writes remote config files before any real validation step.

The renderer has correctness bugs. `replacePathRegex` is emitted with only `regex` and no `replacement`, so regex rewrite routes cannot work as specified. Stream resources emit `{}` at the wrong indentation when only one protocol has resources, producing invalid or misleading YAML. The install script ignores failed Traefik package installation by ending the install command with `|| true`.

The API also exposes direct apply endpoints rather than a consistent sync plan/preview/apply/result/audit model.

## Evidence

- `src/Hashi.Core/Traefik/TraefikUserMiddlewareParser.cs:11-122` is a manual line parser, not YamlDotNet.
- `src/Hashi.Core/Traefik/TraefikConfigValidator.cs:7-70` validates by checking for substrings, not parsing YAML or running Traefik validation.
- `src/Hashi.Infrastructure/Platform/TraefikSyncService.cs:79-106` writes static and dynamic files and marks them applied without a Traefik validation command.
- `src/Hashi.Infrastructure/Platform/TraefikSyncService.cs:235-240` allows package installation to fail with `|| true`.
- `src/Hashi.Core/Traefik/TraefikConfigRenderer.cs:294-298` emits `replacePathRegex` without a `replacement`.
- `src/Hashi.Core/Traefik/TraefikConfigRenderer.cs:362-372` inserts `{}` without indentation under `routers:` or `services:`.
- `src/Hashi.Api/Features/Platform/PlatformEndpoints.cs:141-159` exposes direct Traefik apply endpoints.

## Expected outcome

Traefik config should be rendered atomically, parsed safely, validated with Traefik before replace, compared by hash to avoid identical rewrites, and applied only through a plan/preview/apply/result/audit flow. User middleware YAML should be parsed with YamlDotNet. Regex rewrites should include replacement semantics.

## Fix guidance

Add YamlDotNet and parse all generated/user YAML in tests and validation. Add a remote `traefik check` or container validation step before moving files into place. Fix stream YAML indentation and regex replacement modeling. Convert direct apply into plan/apply or route it through the global sync plan.

## Acceptance criteria

- Invalid user/generated YAML fails before any remote write.
- Traefik package install failure fails install.
- TCP-only and UDP-only rendered YAML parses successfully.
- Regex rewrite includes both regex and replacement.
- Apply creates an auditable sync result and does not rewrite identical files.
