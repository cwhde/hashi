# B12 - Edge SSO still uses custom OIDC token handling

Priority: Critical

Spec conflicts: non-negotiable rule 17 and section 4. OIDC must use proven platform APIs, specifically `Microsoft.AspNetCore.Authentication.OpenIdConnect`, rather than custom token parsing and validation.

## Problem

Edge SSO constructs authorization and token URLs manually, exchanges the authorization code with raw `HttpClient`, and extracts `sub` from `id_token` by base64-decoding the JWT payload. It does not validate the ID token signature, issuer, audience, nonce, or token lifetime through the platform OIDC handler.

The implementation also treats localhost and fake issuers as a built-in bypass inside production code.

## Evidence

- `src/Hashi.Infrastructure/Platform/OidcEdgeAuthService.cs:35-52` manually builds an authorization URL from the configured issuer.
- `src/Hashi.Infrastructure/Platform/OidcEdgeAuthService.cs:141-169` manually posts to `{issuer}/oauth/token` and accepts the response.
- `src/Hashi.Infrastructure/Platform/OidcEdgeAuthService.cs:169` falls back to `ParseIdTokenSubject(payload.IdToken)`.
- `src/Hashi.Infrastructure/Platform/OidcEdgeAuthService.cs:172-191` base64-decodes the ID token payload and reads `sub` without cryptographic validation.
- `src/Hashi.Infrastructure/Platform/OidcEdgeAuthService.cs:147-149` returns a subject directly for fake issuers.
- `src/Hashi.Infrastructure/Platform/OidcEdgeAuthService.cs:199-203` treats `/fake`, `.fake.`, localhost, and 127.0.0.1 as fake issuers.
- `src/Hashi.Api/Hashi.Api.csproj:9-14` has no package reference for `Microsoft.AspNetCore.Authentication.OpenIdConnect`.

## Expected outcome

OIDC login should use the platform OpenID Connect handler or equivalent vetted validation library. Tokens must be validated for signature, issuer, audience, nonce/state binding, expiration, and required claims before creating an Edge SSO session.

## Fix guidance

Move Edge SSO to ASP.NET Core OpenID Connect authentication schemes per provider, or introduce a vetted OIDC client/validation library if dynamic providers require custom scheme registration. Keep fake-provider behavior test-only and outside production code paths.

## Acceptance criteria

- Edge SSO validates ID tokens cryptographically.
- Issuer, audience, nonce, state, and expiration are enforced.
- Production code has no localhost/fake issuer authentication bypass.
- Tests cover invalid signature, wrong issuer, wrong audience, expired token, replayed/missing nonce, and successful provider login.
