# AUD-100: Trailing slash on secret reveal bypasses reauth and secret scope

Priority: P1

Category: Security / authorization path matching

Status: open; source confirmed with independent ASP.NET routing reproduction.

## Finding

The route `/api/vault/secrets/{id}/reveal` also matches with a trailing slash, while the middleware recognizes secret reveal using the raw path's `EndsWith("/reveal")`. For `/reveal/`, both special checks fail. Because it is a GET, RequiredScope falls through to `admin.read` and RequiresReauthentication returns false. The endpoint handler then decrypts and returns a secret if that session's vault is unlocked. Thus a stale reauth session can reveal a secret, and a session lacking secrets.manage is not rejected by that scope guard. This does not bypass ordinary login, IP binding or the need for an unlocked session vault.

## Evidence

- `src/Hashi.Api/Hosting/AdminApiAuthMiddleware.cs:85-98,142-156`: raw suffix check before safe-method return.
- `src/Hashi.Api/Features/Vault/VaultEndpoints.cs`: mapped GET reveal handler has no independent reauth/scope check.
- `work/repros/routing/Program.cs`: independent .NET10 minimal host matches both `/reveal` and `/reveal/` with 200 while the identical suffix predicate changes from true to false. Synthetic endpoint only; no real credentials or secrets used.
- Main spec §8/§9 and admin-session addendum require recent reauth and secrets scope for reveal. Historical A01 is not fully resolved.

## Expected outcome

Authorize the selected endpoint using explicit scope/reauth metadata and policies. If path checks remain, normalize consistently with routing and apply identical checks for all equivalent URLs.

## Acceptance criteria

Both URL forms enforce secrets.manage, fresh passkey reauth and the current session's unlocked vault. Read-only and stale-reauth sessions fail equally for both forms; verify through the actual host.
