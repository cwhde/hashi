# AUD-014: OIDC callback redirects to an untrusted return URL

Priority: P2 — security / redirect validation

Spec reference: implementation spec §§8.4 and 24; authentication redirects must preserve a safe, validated return destination.

## Finding

`GET /api/edge-auth/login` accepts `returnUrl` from the request and passes it into `OidcEdgeAuthService`. The service only substitutes `/` for an empty value; it does not constrain absolute or protocol-relative URLs. The callback later redirects directly to the persisted value after a successful OIDC login. A crafted login link can therefore make Hashi redirect a user to an attacker-controlled site after authentication, lending the redirect the credibility of the trusted Hashi domain.

## Evidence

- `src/Hashi.Api/Features/EdgeAuth/EdgeAuthEndpoints.cs`: the login handler accepts `returnUrl`, uses it for provider selection, and passes it unchanged to `BuildAuthorizationUrlAsync`; the callback returns `TypedResults.Redirect(result.ReturnUrl)`.
- `src/Hashi.Infrastructure/Platform/OidcEdgeAuthService.cs`: `NormalizeReturnUrl` only checks for whitespace and otherwise returns the supplied string; the value is protected and persisted in the correlation cookie, then returned from callback completion.

## Expected outcome

Accept only a local path (or a destination explicitly validated against configured resource domains). Reject or safely replace absolute, protocol-relative, backslash, and otherwise ambiguous destinations. Apply the same validation before provider selection and when persisting correlation state.

## Acceptance criteria

- An external absolute URL cannot be returned as the post-login redirect.
- Protocol-relative and parser-ambiguous variants are also rejected or reduced to `/`.
- Valid local paths, including query strings and fragments where appropriate, continue to work.
- Validation is covered at the URL normalization boundary and in the endpoint flow.
