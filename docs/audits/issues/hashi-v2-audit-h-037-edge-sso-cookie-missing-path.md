# H-037: OidcEdgeAuthService Session Cookie Missing Explicit Path

**Priority:** Medium
**Conflict Type:** bad_implementation
**Spec Reference:** Main Spec §11, §9

## Description

In `OidcEdgeAuthService.BuildSessionCookie()` (around lines 296-306), the edge SSO session cookie is created without explicitly setting the `Path` property:

```csharp
// OidcEdgeAuthService.cs
var cookieOptions = new CookieOptions
{
    HttpOnly = true,
    Secure = true,
    SameSite = SameSiteMode.Lax,
    Expires = expiresAtUtc.UtcDateTime,
    Domain = domain,  // root domain like .example.com
    // Path not set — inherits request path
};
```

Because `Path` is not set, the browser defaults to the request path. If the user logs in from `/api/edge-auth/login?returnUrl=/some-app/dashboard`, the cookie's Path may be `/api/edge-auth/` or `/some-app/`, making it unavailable on other paths. This means the edge SSO session would not be detected on other subdomains or paths, breaking the cross-subdomain single-sign-on behavior mandated by spec §11: "Session cookie domain is the root domain, for example `.example.com`, so one login covers subdomains under that root."

## Evidence

```csharp
// OidcEdgeAuthService.cs — BuildSessionCookie
var cookieOptions = new CookieOptions
{
    HttpOnly = true,
    Secure = true,
    SameSite = SameSiteMode.Lax,
    Expires = expiresAtUtc.UtcDateTime,
    Domain = domain,
    // Path = ? — NOT SET
};
```

## Expected Outcome

The edge SSO session cookie should have `Path = "/"` to ensure the cookie is sent for all requests under the configured root domain. This is consistent with the spec's requirement that "one login covers subdomains under that root."

## Fix Guidance

1. Add `Path = "/"` to the `CookieOptions` in `BuildSessionCookie`.
2. Verify this does not conflict with any cookie path isolation requirements.
3. Add a test verifying the cookie is accessible across different paths under the same domain.

## Acceptance Criteria

- [ ] Edge SSO session cookie has `Path = "/"`
- [ ] Cookie is sent on requests to different paths under the same root domain
- [ ] One SSO login covers all resources on the root domain
- [ ] Cookie path configuration does not interfere with admin cookie scope
