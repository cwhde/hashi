# H-038: SecurityDecisionService Inconsistent Priority Ordering Between Rule Types

**Priority:** Medium
**Conflict Type:** wrong_implementation
**Spec Reference:** Main Spec §6 (Resource Rule Model)

**Status:** Fixed
**Branch:** h/backend-quality

## Description

`SecurityDecisionService` in `src/Hashi.Infrastructure/Platform/SecurityDecisionService.cs` evaluates resource rules and global edge-auth rules with opposite priority ordering:

- **Resource rules** (line ~291): Ordered by `Priority` **descending** (higher number = higher priority)
- **Global edge-auth rules** (line ~348): Ordered by `Priority` **ascending** (lower number = higher priority)

The spec §6 states: "Rules are evaluated by priority, higher first." This is unambiguous — higher priority value means evaluated first. However, the implementation has inconsistent semantics between rule types, which means:
1. A resource rule with `Priority = 100` is evaluated *before* one with `Priority = 50`.
2. A global edge rule with `Priority = 100` is evaluated *after* one with `Priority = 50`.

This inconsistency creates confusion and potential misconfiguration where users expect consistent behavior across rule types.

## Evidence

```csharp
// ResourceRulesAsync — descending (line ~291)
.OrderByDescending(r => r.Priority)

// EvaluateGlobalEdgeRulesAsync — ascending (line ~348)
.OrderBy(r => r.Priority)
```

## Expected Outcome

Both resource rules and global edge-auth rules should use the same priority ordering. Per spec §6, the correct behavior is descending order (higher priority first).

## Fix Guidance

1. Change `EvaluateGlobalEdgeRulesAsync` to use `OrderByDescending(r => r.Priority)` to match resource rule ordering.
2. Add unit tests verifying that higher-priority rules are evaluated before lower-priority rules for both rule types.
3. Document the priority semantics clearly in code comments.

## Acceptance Criteria

- [ ] Both rule types use `OrderByDescending` on `Priority`
- [ ] Test: Priority-100 rule evaluates before Priority-50 rule for global edge requirements
- [ ] Test: Priority ordering is consistent across `EvaluateGlobalEdgeRulesAsync` and `EvaluateResourceRulesAsync`
- [ ] Priority semantics documented in code comments
