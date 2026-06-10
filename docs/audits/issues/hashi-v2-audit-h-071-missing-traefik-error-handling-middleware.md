# H-071: Missing Traefik Error Handling Middleware

**Priority:** Medium
**Conflict Type:** missing_implementation
**Spec Reference:** Main Spec §10.4

**Status:** Not Started
**Branch:** 

## Description

The generated default middlewares include HTTP-to-HTTPS redirect, security headers, compression, forward auth, WAF, and baseline rate limit — but there is no error handling middleware. The spec lists "Error handling, optional" as one of the default middlewares. Without it, Traefik returns its default plain-text error pages for 5xx responses, which are unstyled and unhelpful.

## Evidence

- `TraefikConfigRenderer.BuildDefaultMiddlewares()` generates `hashi-redirect-https`, `hashi-security-headers`, `hashi-compress`, `hashi-forward-auth-*`, `hashi-rate-limit`
- No error handling middleware (e.g., custom 5xx error pages) exists in the default middleware chain

## Expected Outcome

An error handling middleware that renders styled error pages for 5xx responses, matching the Hashi visual theme. This should be optional (can be disabled per resource or globally).

## Fix Guidance

1. Add a Traefik error middleware that captures 5xx responses and serves a custom error page.
2. The error page should be minimal, match the Hashi theme, and not expose internal details.
3. Make it optional via a setting.

## Acceptance Criteria

- [ ] Traefik generates a `hashi-errors` middleware in `00-hashi-core.yml`
- [ ] 5xx responses show a styled error page instead of Traefik default
- [ ] Error handling middleware can be disabled per resource or globally
