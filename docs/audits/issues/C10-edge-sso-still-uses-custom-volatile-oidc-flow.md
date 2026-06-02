# C10 - Edge SSO still uses a custom volatile OIDC flow

Priority: High

Spec conflicts: non-negotiable rule 17 and the architecture section require critical OIDC behavior to use proven libraries or platform APIs, specifically `Microsoft.AspNetCore.Authentication.OpenIdConnect`. Edge SSO also requires one or more OIDC providers, cross-subdomain session cookies, and configurable session policy.

## Problem

Edge SSO improved since the previous audit, but it is still a hand-written OIDC implementation rather than the platform OpenID Connect handler required by the spec. It manually builds authorization URLs, stores state/nonce in a static in-memory dictionary, manually exchanges codes, manually fetches discovery/JWKS, and manually validates ID tokens.

The in-memory pending-login state is lost on restart and is not shared across instances. The authorization request does not use PKCE. Production code still contains a fake/unsigned issuer bypass path controlled by configuration. Session policy remains partial as well: maximum length exists, but idle timeout and remember-device behavior are not represented.

## Evidence

- `docs/implementation-spec/hashi-v2-implementation-spec.md:35-42` requires critical OIDC behavior to use proven libraries or platform APIs.
- `docs/implementation-spec/hashi-v2-implementation-spec.md:56-63` names `Microsoft.AspNetCore.Authentication.OpenIdConnect` for upstream identity providers.
- `docs/implementation-spec/hashi-v2-implementation-spec.md:435-441` defines Edge SSO provider, cookie-domain, session length, idle timeout, and remember-device requirements.
- `src/Hashi.Infrastructure/Platform/OidcEdgeAuthService.cs:26` stores pending OIDC logins in a static dictionary.
- `src/Hashi.Infrastructure/Platform/OidcEdgeAuthService.cs:40-62` manually builds the authorization URL and omits PKCE parameters.
- `src/Hashi.Infrastructure/Platform/OidcEdgeAuthService.cs:84-98` validates state from the in-memory dictionary.
- `src/Hashi.Infrastructure/Platform/OidcEdgeAuthService.cs:150-180` manually exchanges the authorization code.
- `src/Hashi.Infrastructure/Platform/OidcEdgeAuthService.cs:183-229` manually validates the ID token and nonce.
- `src/Hashi.Infrastructure/Platform/OidcEdgeAuthService.cs:284-291` keeps an unsigned fake issuer bypass in production code paths.
- `rg -n "Microsoft.AspNetCore.Authentication.OpenIdConnect|AddOpenIdConnect|OpenIdConnect" src tests` returns no implementation usage.

## Expected outcome

Edge SSO should use ASP.NET Core OpenID Connect or an equivalent proven OIDC client flow with durable correlation/state, nonce validation, PKCE where applicable, and the session settings required by the spec.

## Fix guidance

Replace the manual login/callback flow with `AddOpenIdConnect` schemes or a comparable vetted OIDC client library. Store correlation through framework cookies or durable state appropriate for the deployment. Add PKCE, remove fake unsigned issuer support from production code, and add session idle timeout and remember-device settings.

## Acceptance criteria

- Edge SSO uses platform OIDC middleware or an approved OIDC client library for authorization, callback, state, nonce, token exchange, and token validation.
- Pending login state survives process restarts or uses standard secure correlation cookies that do not require server memory.
- Authorization requests include PKCE when supported.
- Unsigned fake issuer support is removed from production paths or isolated to test-only code.
- Session length, idle timeout, remember-device policy, and root-domain cookie behavior are covered by tests.
