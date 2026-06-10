# H-014: Traefik Dynamic Config File Naming Deviation

**Priority:** Medium
**Conflict Type:** wrong_implementation
**Spec Reference:** Main Spec §10.3

**Status:** Fixed
**Branch:** h/spec-compliance-1

## Description

The implementation spec defines specific naming for Traefik dynamic config files:
```
00-hashi-core.yml
10-hashi-http-resources.yml
20-hashi-stream-resources.yml
30-user-middlewares.yml
40-hashi-security.yml
90-hashi-health.yml
```

Examining the `TraefikHostStateEntity` in `ExtendedPlatformEntities.cs`, the entity stores `StaticConfigPath` and `DynamicConfigPath` as single paths:
```csharp
public string StaticConfigPath { get; set; } = "/etc/hashi/traefik/traefik.yml";
public string DynamicConfigPath { get; set; } = "/etc/hashi/traefik/dynamic/http.yml";
```

The entity only stores a single dynamic config path, not the six separate files specified in the spec. The `TraefikConfigRenderer` in `Hashi.Core/Traefik/` would need to generate multiple files, but the entity model only tracks one path.

This could mean:
1. The renderer generates all files but the entity only tracks the main one
2. The implementation uses a single combined file instead of separate files
3. The entity model is incomplete

Without reading the full `TraefikConfigRenderer.cs`, I cannot confirm which approach is used. However, the entity model suggests a deviation from the spec's six-file approach.

## Evidence

Spec requirement (§10.3):
```
Hashi writes separate dynamic files:
- 00-hashi-core.yml
- 10-hashi-http-resources.yml
- 20-hashi-stream-resources.yml
- 30-user-middlewares.yml
- 40-hashi-security.yml
- 90-hashi-health.yml
```

Entity model:
```csharp
// ExtendedPlatformEntities.cs
public string DynamicConfigPath { get; set; } = "/etc/hashi/traefik/dynamic/http.yml";
```

Single path instead of six separate paths.

## Expected Outcome

- Traefik dynamic configs are generated as six separate files
- Each file is tracked independently
- File naming matches spec exactly

## Fix Guidance

1. Verify how `TraefikConfigRenderer` generates files
2. If using a single file, document the deviation
3. If using multiple files, update the entity model to track all paths
4. Ensure the `30-user-middlewares.yml` is user-editable through the UI

## Acceptance Criteria

- [ ] Traefik dynamic configs use the specified file naming
- [ ] Each config file is generated separately
- [ ] `30-user-middlewares.yml` is user-editable
- [ ] Entity model tracks all generated files
