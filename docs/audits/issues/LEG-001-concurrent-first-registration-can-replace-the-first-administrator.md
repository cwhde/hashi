# LEG-001: Concurrent first registration can replace the first administrator

Priority: P2

Category: Legacy security / account creation

Status: open; manual source review. Applies to the separately deployed legacy artifact, not the active V2 process.

## Finding

AuthService.register checks for an existing account, then awaits bcrypt before saving the new account. Two unauthenticated first-registration requests can both pass the check and save different administrators; the last write wins. Login has no rate limiter, session cleanup is never scheduled and password/config changes do not revoke existing sessions.

## Evidence

hashi.old/src/services/auth.js:72-100 and session methods; hashi.old/src/routes/auth.js. Applies to separately deployed legacy server.

## Expected outcome and acceptance criteria

Serialize first registration, guard bootstrap exposure, bound authentication attempts and implement explicit session expiry/revocation/cleanup. Concurrent bootstrap must produce exactly one account.
