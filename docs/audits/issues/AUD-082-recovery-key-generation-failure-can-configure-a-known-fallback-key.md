# AUD-082: Recovery-key generation failure can configure a known fallback key

Priority: P1

Category: Security / recovery key generation

Status: open; confirmed by manual source tracing.

## Finding

PasskeyVaultStep catches recovery generation failure and assigns the literal offline placeholder to recoveryKey. Configure is enabled based on passkey/confirmation and does not reject this placeholder. The vault API/service accepts any supplied recovery string and wraps the root using HKDF directly over it. If the generation call fails transiently and later setup succeeds, the vault can be protected by a publicly known string rather than a random recovery key. The verification call then validates that same weak key.

## Evidence

web/src/lib/components/setup/steps/PasskeyVaultStep.svelte:40-48,85-98,164-166; src/Hashi.Infrastructure/Auth/VaultService.cs:45-72; src/Hashi.Infrastructure/Crypto/KeyDerivation.cs:11-12

## Expected outcome

Represent generation failure separately, disable setup until a valid cryptographic key exists, and enforce recovery-key requirements server side.

## Acceptance criteria

Generation failure cannot create a vault; placeholder/empty/weak keys are rejected; retry generates a fresh secure key and supports setup.

No live credentials, host scripts or services were changed for this review.
