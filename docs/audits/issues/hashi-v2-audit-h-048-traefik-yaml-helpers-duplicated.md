# H-048: TraefikConfigValidator and TraefikUserMiddlewareParser Duplicate YAML Navigation Helpers

**Priority:** Low
**Conflict Type:** bad_implementation
**Spec Reference:** Main Spec §4, §10

## Description

Both `TraefikConfigValidator` (`src/Hashi.Core/Traefik/TraefikConfigValidator.cs`) and `TraefikUserMiddlewareParser` (`src/Hashi.Core/Traefik/TraefikUserMiddlewareParser.cs`) contain identical implementations of `TryGetMapping` and `TryGetNode` methods:

```csharp
// Both files contain these duplicated helpers:
private static bool TryGetMapping(YamlNode node, [NotNullWhen(true)] out YamlMappingNode? mapping)
private static bool TryGetNode(YamlMappingNode mapping, string key, [NotNullWhen(true)] out YamlNode? value)
```

This violates the DRY principle and creates a maintenance burden: any fix to YAML parsing logic must be applied in two places. If one file is updated and the other is missed, the validator and parser can diverge in behavior.

## Evidence

- `TraefikConfigValidator.cs` — Contains `TryGetMapping` and `TryGetNode`
- `TraefikUserMiddlewareParser.cs` — Contains identical `TryGetMapping` and `TryGetNode`

Identical method signatures, identical implementations.

## Expected Outcome

The YAML navigation helpers should be extracted into a shared internal utility class (e.g., `YamlNavigationHelpers`) referenced by both files.

## Fix Guidance

1. Create a shared internal class `YamlNavigationHelpers` in `Hashi.Core/Traefik/`.
2. Move `TryGetMapping` and `TryGetNode` there as `internal static` methods.
3. Reference from both `TraefikConfigValidator` and `TraefikUserMiddlewareParser`.
4. Verify both classes continue to function identically.

## Acceptance Criteria

- [ ] No duplicated `TryGetMapping`/`TryGetNode` methods exist across files
- [ ] Both validator and parser share the same YAML helpers
- [ ] All existing tests pass
- [ ] YAML parsing behavior is unchanged
