# H-035: SameSite Cookie Mode Not Explicitly Set for Auth and CSRF Cookies

**Priority:** Medium
**Conflict Type:** bad_implementation
**Spec Reference:** Main Spec §9

## Description

The admin authentication cookie (`hashi.session`) and CSRF cookie (`hashi.csrf`) are configured in `Program.cs` without explicitly setting the `SameSite` property. On .NET 10, the default `SameSite` value is `Lax`, which may be insufficient for certain deployment scenarios:

- If the admin SPA is accessed from a different origin (e.g., via Traefik proxy with different port/domain), `SameSite=Lax` blocks the cookie from being sent on cross-origin requests.
- `SameSite=Strict` would be more secure but could break redirect-based flows.
- `SameSite=None` would work cross-origin but requires `Secure=true` and introduces CSRF risk.

The spec §9 states: "Admin sessions use secure, HTTP-only cookies. CSRF protection on unsafe methods." Without explicit `SameSite` configuration, the actual security posture depends on the .NET default, which could change between versions.

## Evidence

```csharp
// Program.cs — cookie auth setup
options.Cookie.HttpOnly = true;
options.Cookie.SecurePolicy = CookieSecurePolicy.Always;
// SameSite not set — defaults to Lax

// Antiforgery setup
options.Cookie.HttpOnly = true;
// SameSite not set — defaults to Lax
```

## Expected Outcome

The `SameSite` mode should be explicitly set for both cookies:

- `hashi.session`: `SameSite=Strict` for maximum security (admin UI is same-origin by design)
- `hashi.csrf`: `SameSite=Strict` since CSRF tokens are read via JS on the same origin

## Fix Guidance

1. Add `options.Cookie.SameSite = SameSiteMode.Strict;` to the cookie auth configuration.
2. Add `options.Cookie.SameSite = SameSiteMode.Strict;` to the antiforgery configuration.
3. Document the SameSite setting in the security settings section.

## Acceptance Criteria

- [ ] `hashi.session` cookie has `SameSite=Strict`
- [ ] `hashi.csrf` cookie has `SameSite=Strict`
- [ ] CSRF protection still functions correctly after change
- [ ] Admin login and session management work end-to-end
