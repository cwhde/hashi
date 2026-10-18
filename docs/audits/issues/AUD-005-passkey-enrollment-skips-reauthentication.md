# AUD-005 - Adding a passkey does not require recent reauthentication

Priority: High (P1)

Category: Security / account credential management

Specification reference: `docs/implementation-spec/hashi-v2-admin-session-token-management-addendum.md`, sections 10 and 11 (security scopes and passkey reauthentication); base specification section 9 (passkey admin authentication).

## Problem

An authenticated session can register another passkey without recent passkey reauthentication. The registration endpoints are mutating `/api/auth/passkeys/register/*` routes, but `AdminApiAuthMiddleware` does not classify registration as a `security.manage` operation or as requiring recent reauthentication. `afterSetup=true` explicitly enables registration after setup, so the server accepts this credential-enrollment path even though the current UI only exposes it during setup. A stolen session can be used to enroll an attacker-controlled authenticator and establish durable account access.

## Evidence

- `AdminApiAuthMiddleware.RequiredScope` only assigns `security.manage` to session and passkey deletion operations; passkey registration falls through to ordinary `admin.write`.
- `AdminApiAuthMiddleware.RequiresReauthentication` handles passkey deletion but not registration.
- `AuthEndpoints` allows authenticated registration after setup when the caller supplies `afterSetup=true`, and completion checks only that the caller is authenticated during setup.
- The admin-session addendum requires high-risk operations to use recent passkey reauthentication and says privilege changes are allowed only after successful reauthentication.

## Expected outcome

Post-setup credential enrollment is an explicitly supported, security-scoped operation that requires recent passkey reauthentication. The enrollment challenge is bound to the initiating session and cannot be completed from another session.

## Acceptance criteria

- Require `security.manage` and recent passkey reauthentication for post-setup passkey registration.
- Bind registration challenge completion to the same valid session that initiated it.
- Keep first-passkey setup registration available only in the incomplete setup flow.
- Add integration tests for denied registration without reauthentication, successful registration after reauthentication, and cross-session challenge completion.
