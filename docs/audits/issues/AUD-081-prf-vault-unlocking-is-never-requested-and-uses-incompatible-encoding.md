# AUD-081: PRF vault unlocking is never requested and uses incompatible encoding

Priority: P1

Category: Missing feature / passkey vault

Status: open; confirmed by manual source tracing.

## Finding

Neither backend WebAuthn options nor frontend option handling requests the PRF extension or supplies its evaluation salt. Registration stores a fresh PrfSalt only after the ceremony, and it is never used to request PRF on login. extractPrfOutput therefore has no requested output to extract, so supported authenticators fall back to recovery-key-only vault access. If output is supplied, the frontend serializes unpadded base64url while both API consumers use Convert.FromBase64String, which rejects that encoding for many outputs.

## Evidence

web/src/lib/auth/webauthn.ts:17-49,67-84,115-121; src/Hashi.Infrastructure/Auth/PasskeyAuthService.cs:60-97,115-137; src/Hashi.Api/Features/Auth/AuthEndpoints.cs:173-175; src/Hashi.Api/Features/Vault/VaultEndpoints.cs:40-42

## Expected outcome

Implement stable per-credential PRF evaluation during supported ceremonies and one consistent binary encoding; distinguish browser support from credential support.

## Acceptance criteria

A real PRF-capable credential configures a wrap and unlocks it on a fresh login using the same evaluation input. Unsupported credentials use the documented recovery fallback.

No live credentials, host scripts or services were changed for this review.
