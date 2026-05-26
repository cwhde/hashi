# A01 - Admin auth and CSRF middleware expose protected endpoints

Priority: Critical

Spec conflicts: non-negotiable rules 15, 18, 21, and section 30. Unsafe admin endpoints require CSRF protection. High-risk endpoints require recent reauthentication. Setup must not be marked configured or allow privileged operations until passkey and vault setup are complete.

## Problem

The admin auth middleware treats all `/api/auth` routes as always public, not just login, bootstrap, CSRF, and passkey ceremony endpoints. That makes authenticated account management and reauthentication endpoints public at the middleware layer. The setup-phase bypass also allows unauthenticated `/api/vault`, `/api/settings`, and `/api/activity` calls before setup is complete.

The CSRF middleware compounds this by exempting every unsafe `/api/auth` request from CSRF validation.

Secret reveal is implemented as a `GET`, while `RequiresReauthentication` returns early for safe methods. That means the intended reauth check for `/api/vault/secrets/{id}/reveal` is never applied.

## Evidence

- `src/Hashi.Api/Hosting/AdminApiAuthMiddleware.cs:14-21` includes `new("/api/auth")` in `AlwaysPublicPrefixes`.
- `src/Hashi.Api/Hosting/AdminApiAuthMiddleware.cs:42-47` bypasses auth for setup-phase paths.
- `src/Hashi.Api/Hosting/AdminApiAuthMiddleware.cs:195-199` includes `/api/vault`, `/api/settings`, and `/api/activity` in the setup-phase bypass.
- `src/Hashi.Api/Hosting/AdminApiAuthMiddleware.cs:74-84` only checks reauth after rejecting safe methods.
- `src/Hashi.Api/Hosting/AdminCsrfMiddleware.cs:29-34` skips CSRF for all `/api/auth` endpoints.
- `src/Hashi.Api/Features/Auth/AuthEndpoints.cs:69-89` exposes passkey listing/deletion under `/api/auth/passkeys`.
- `src/Hashi.Api/Features/Vault/VaultEndpoints.cs:107-118` reveals a secret through `GET /api/vault/secrets/{id}/reveal`.

## Expected outcome

Only explicitly public auth endpoints should be public. Passkey listing/deletion, reauthentication, session-sensitive auth operations, vault secret creation/reveal, settings writes, and activity history must require an authenticated admin session. Unsafe admin calls must require CSRF. Secret reveal must require recent passkey reauthentication regardless of HTTP verb.

## Fix guidance

Replace broad prefix bypasses with an allowlist of exact public endpoints and methods. Make secret reveal a `POST` or special-case it before the safe-method return. Restrict the setup bypass to the minimum bootstrap/status endpoints and require bootstrap/passkey session for setup mutations as appropriate.

## Acceptance criteria

- Anonymous requests to `/api/auth/passkeys`, `/api/auth/passkeys/{id}`, `/api/auth/reauthenticate`, `/api/vault/secrets`, `/api/settings/general`, and `/api/activity/audit` fail after middleware.
- Unsafe `/api/auth` account-management endpoints require CSRF unless they are part of the initial login/registration ceremony.
- Secret reveal requires a recent passkey reauthentication.
- Tests cover anonymous, bootstrap, passkey, stale-reauth, and recent-reauth cases.
