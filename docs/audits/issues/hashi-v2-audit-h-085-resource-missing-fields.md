# H-085: ResourceEntity Missing AdGuardRewriteVisibility, ExplicitRoutingOverride, SecurityProfile

**Priority:** Medium
**Conflict Type:** missing_implementation
**Spec Reference:** §6 (Resources must have AdGuardRewriteVisibility, ExplicitRoutingOverride, SecurityProfile)

## Description

`ResourceEntity` lacks `AdGuardRewriteVisibility` (no toggle for AdGuard rewrite per resource), `ExplicitRoutingOverride` (no way to override auto-detected routing), and `SecurityProfile` (only has separate `ForwardAuthPolicy` and `WafMode`, no unified profile).

## Evidence

- `ResourceEntity` has no `AdGuardRewriteVisibility` or equivalent field
- No `ExplicitRoutingOverride` field exists
- Security settings are split across `ForwardAuthPolicy` and `WafMode` with no unified profile concept

## Expected Outcome

Resources should have `AdGuardRewriteEnabled` boolean, `ExplicitRoutingOverride` string, and `SecurityProfileName` string fields. Security profile should bundle forward-auth, WAF, and rate-limit settings.

## Fix Guidance

1. Add `AdGuardRewriteEnabled` boolean field to `ResourceEntity`.
2. Add `ExplicitRoutingOverride` string field to `ResourceEntity`.
3. Add `SecurityProfileName` string field to `ResourceEntity`.
4. Create a `SecurityProfileEntity` that bundles forward-auth, WAF, and rate-limit settings.

## Acceptance Criteria

- [ ] Resources can control AdGuard rewrite visibility
- [ ] Resources can specify explicit routing override
- [ ] Security profile bundles forward-auth, WAF, and rate-limit settings
