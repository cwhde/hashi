# C13 - Admin and edge cookies can be issued without the secure flag

Priority: Medium

Spec conflicts: section 9 requires admin sessions to use secure, HTTP-only cookies. Edge SSO requires cross-subdomain session cookies for protected resources.

## Problem

Hashi configures admin and CSRF cookies with `CookieSecurePolicy.SameAsRequest`, and Edge SSO sets the edge session cookie's `Secure` flag from `context.Request.IsHttps`. If a request reaches Hashi over HTTP, those cookies can be issued without the `Secure` flag.

The setup flow eventually verifies HTTPS, but the cookie policy itself does not enforce the spec's secure-cookie requirement. This also leaves behavior dependent on proxy forwarding correctness: if TLS terminates before Hashi and request scheme forwarding is misconfigured, cookies can be downgraded.

## Evidence

- `docs/implementation-spec/hashi-v2-implementation-spec.md:421-425` requires secure, HTTP-only admin session cookies.
- `docs/implementation-spec/hashi-v2-implementation-spec.md:435-441` requires Edge SSO cross-subdomain session cookies.
- `src/Hashi.Api/Program.cs:56-64` configures `hashi.session` as HTTP-only but `SecurePolicy = CookieSecurePolicy.SameAsRequest`.
- `src/Hashi.Api/Program.cs:81-87` configures the CSRF cookie with the same secure policy.
- `src/Hashi.Infrastructure/Platform/OidcEdgeAuthService.cs:300-305` sets the edge session cookie `Secure` flag to `context.Request.IsHttps`.

## Expected outcome

Admin, CSRF, and edge SSO cookies should be secure by default in production and should not silently lose the `Secure` flag due to request scheme.

## Fix guidance

Use `CookieSecurePolicy.Always` for admin and CSRF cookies outside explicit local-development exceptions. For Edge SSO, set `Secure = true` unless an explicit development-only setting permits insecure localhost cookies. Ensure forwarded headers are configured for reverse-proxy deployments and add tests that cookies are secure.

## Acceptance criteria

- Production admin session cookies are always `Secure` and `HttpOnly`.
- Production CSRF cookies are always `Secure`.
- Production edge SSO cookies are always `Secure` and `HttpOnly`.
- Local development exceptions, if needed, are explicit and cannot be enabled accidentally in production.
- Tests cover cookie flags for admin login, CSRF token issuance, and edge SSO callback.
