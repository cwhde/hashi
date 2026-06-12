# H-030: PasskeyAuthService Skips Server-Side Attestation Verification

**Priority:** Critical
**Conflict Type:** bad_implementation
**Spec Reference:** Main Spec §7.8, §9; Non-Negotiable Rule Set §3 (#17 use proven libraries)

**Status:** Fixed
**Branch:** h/security-1

## Description

In `PasskeyAuthService.CompleteRegistrationAsync()` (line 65) and `CompleteLoginAsync()` (line 120), the FIDO2 server-side verification callbacks always return `true`:

```csharp
var success = await fido2.MakeNewCredentialAsync(
    attestation,
    originalOptions,
    (_, _) => Task.FromResult(true));  // ← no verification
```

```csharp
var success = await fido2.MakeAssertionAsync(
    assertion,
    originalOptions,
    stored.PublicKey,
    stored.SignCount,
    (_, _) => Task.FromResult(true));  // ← no verification
```

The `fido2-net-lib` library expects these callbacks to perform actual verification steps such as:
- Checking the credential is not already registered (registration)
- Validating the authenticator attestation trust path
- Verifying the authenticator's sign count has increased (authentication)

By always returning `true`, the server accepts any WebAuthn credential from any authenticator without any server-side validation beyond what the library does internally. This effectively disables a layer of the WebAuthn security model.

## Evidence

- `PasskeyAuthService.cs:62-65` — `CompleteRegistrationAsync` callback always `true`
- `PasskeyAuthService.cs:115-120` — `CompleteLoginAsync` callback always `true`

The spec §7.8 requires: "Browser attempts WebAuthn PRF support. Hashi creates a vault root key. Vault root key is wrapped by passkey-derived key if PRF is available." This implies a strong security model around passkey registration. Skipping attestation verification undermines this.

## Expected Outcome

Registration callback should:
1. Verify the credential ID is not already registered (idempotency check)
2. Optionally verify attestation if `AttestationConveyancePreference` is not `None`
3. Store any needed metadata about the authenticator

Assertion callback should:
1. Verify the credential exists and is active
2. Check the credential is not revoked
3. Verify the user is authorized via the credential

## Fix Guidance

1. Implement proper registration callback: check existing credential IDs, validate attestation format when present.
2. Implement proper assertion callback: verify credential ownership, check revocation status.
3. Follow the `fido2-net-lib` documentation examples for proper callback implementation patterns.
4. At minimum, for registration: check `IsCredentialIdUniqueToUserAsync` against the database.
5. For assertion: verify the credential ID corresponds to an active, non-revoked passkey.

## Acceptance Criteria

- [ ] Registration callback performs credential uniqueness check against database
- [ ] Registration callback can validate attestation when `AttestationConveyancePreference` is not `None`
- [ ] Assertion callback verifies credential exists and is not revoked
- [ ] Malicious authenticator with unknown key cannot register
- [ ] Revoked/deleted passkey credential cannot authenticate
