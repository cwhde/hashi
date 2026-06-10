# H-068: No Default Per-Resource OIDC Provider

**Priority:** High
**Conflict Type:** missing_implementation
**Spec Reference:** Main Spec §9

**Status:** Not Started
**Branch:** 

## Description

There is no "default OIDC provider" concept. `OidcProviderEntity` has no `IsDefault` flag. `ResourceEntity` has no `OidcProviderId` field. Login always requires explicit `providerId`. The spec requires per-resource OIDC provider selection and the ability to set a default provider so Hashi can redirect directly to it.

## Evidence

- `OidcProviderEntity` has no `IsDefault` boolean
- `ResourceEntity` has no `OidcProviderId` FK
- Edge-auth login endpoint requires `providerId` parameter with no fallback to a default

## Expected Outcome

Resources can specify which OIDC provider to use (or use a default). A global default OIDC provider can be configured. When a default is set and a resource doesn't specify a provider, Hashi redirects to the default provider.

## Fix Guidance

1. Add `IsDefault` boolean to `OidcProviderEntity` (only one can be default).
2. Add `OidcProviderId` nullable FK to `ResourceEntity`.
3. In edge-auth login, if no explicit `providerId`, check the resource's `OidcProviderId`, then fall back to the global default provider.

## Acceptance Criteria

- [ ] Admin can set a default OIDC provider
- [ ] Resources can specify an explicit OIDC provider
- [ ] Edge-auth login redirects to default provider when no explicit provider specified
- [ ] Default provider selection is available in setup/settings
