# A05 - Edge SSO/OIDC fails open and bypasses standard token validation

Priority: Critical

Spec conflicts: non-negotiable rule 17; Edge SSO sections 11 and 13. SSO required resources must fail closed when Hashi cannot evaluate. OIDC must use proven libraries or platform APIs.

## Problem

Forward auth allows traffic when no enabled OIDC provider exists, even for `sso_required` or strict-mode requests. That violates fail-closed behavior for protected resources.

Resource rules that require auth/challenge are evaluated before validating an existing edge session, so authenticated users can be challenged again or looped if a matching rule exists.

OIDC is implemented manually. The callback accepts missing or unknown state instead of rejecting it. The code parses the ID token payload without validating signature, issuer, audience, expiry, or nonce. It also has fake issuer bypass behavior tied to localhost/fake issuers in production code.

GeoIP rule validation exists but is not used when creating or updating rules, so unavailable GeoIP data does not prevent enabling country/region/ASN rules.

## Evidence

- `src/Hashi.Infrastructure/Platform/EdgeAuthService.cs:87-91` returns `allow` when no enabled OIDC providers exist.
- `src/Hashi.Infrastructure/Platform/EdgeAuthService.cs:34-40` evaluates resource rules before session validation at lines 82-85.
- `src/Hashi.Infrastructure/Platform/EdgeAuthService.cs:139-145` challenges for `require_adaptive_challenge` and `pass_to_auth` without checking existing sessions first.
- `src/Hashi.Infrastructure/Platform/OidcEdgeAuthService.cs:55-88` treats `state` as optional and does not fail when it is missing or unknown.
- `src/Hashi.Infrastructure/Platform/OidcEdgeAuthService.cs:150-167` hand-rolls token exchange and subject extraction.
- `src/Hashi.Infrastructure/Platform/OidcEdgeAuthService.cs:170-201` parses ID token JSON without cryptographic validation and has fake issuer behavior.
- `src/Hashi.Infrastructure/Platform/OidcProviderAdminService.cs:116-165` creates/updates rules without calling `ValidateRuleMatchJson`.

## Expected outcome

SSO required and strict-mode resources must fail closed when auth evaluation is unavailable or misconfigured. OIDC must use `Microsoft.AspNetCore.Authentication.OpenIdConnect` or equivalent validated token handling. State must be mandatory and single-use. GeoIP-dependent rules must be invalid and disabled when GeoIP data is unavailable.

## Fix guidance

Move session validation before resource rules that only require auth/challenge, or make those rules session-aware. Replace manual OIDC parsing with platform OpenID Connect handling. Reject callbacks with missing/unknown/expired state. Enforce GeoIP validation in rule create/update paths.

## Acceptance criteria

- `sso_required` with no provider returns deny/challenge fail-closed, not allow.
- Authenticated edge sessions do not get re-challenged by matching auth rules.
- OIDC callback fails without valid state and validated tokens.
- GeoIP match rules cannot be enabled unless the required GeoIP database is available.
