# AUD-096: Deployment key can decrypt session-only secrets and purpose keys are never separated

Priority: P1

Category: Security / vault capability isolation

Status: open; confirmed by manual source trace and independent synthetic reproduction.

## Finding

VaultService.SetupAsync encrypts the admin root key under the deployment-derived service-sync key. Every secret DEK also has an admin-root wrap, including generic session-only secrets with IsServiceSyncEligible=false. A database copy plus the deployment key can therefore decrypt the admin root, any admin-wrapped DEK and every secret without PRF/recovery/session access. Refusing the normal DecryptForServiceSyncAsync method does not provide the specified cryptographic isolation. In addition InitializePurposeKey has no production caller; GetPurposeWrapKeyOrThrow falls back to the same global wrap key for every purpose. StoreAsync adds purpose wraps whenever a purpose tag exists regardless of eligibility, and DecryptForPurposeAsync does not check eligibility/class. Stored class labels and purpose tags do not establish separate capabilities.

## Evidence

src/Hashi.Infrastructure/Auth/VaultService.cs:91-99,179-192; Auth/SecretRecordService.cs:34-46,120-146,169-194; Auth/VaultSessionState.cs:107-136; Crypto/KeyDerivation.cs:22-23. Repository search finds InitializePurposeKey only at declaration.

Main specification section8 requires purpose-specific vault keys and limits service-sync decryption to reconciliation secrets. Historical A02 and H059 are only partly addressed; do not mark their entire boundary verified.

## Verification

Independent synthetic executable at work/repros/envelope uses the repository AES-GCM/HKDF helpers and reproduces its storage chain: recovery succeeded with only DB blobs plus deployment key. It does not access a real DB or invoke the vault services; source establishes that they persist this chain.

## Expected outcome

Remove deployment-key access to admin-only root material, use separately scoped wrap keys and explicit eligibility/class checks, and design a forward migration that preserves recoverability. Decide the minimum runtime purposes deliberately rather than depending on labels or fallbacks.

## Acceptance criteria

With database ciphertexts and the service deployment key, a synthetic session-only secret remains undecryptable; authorized runtime secrets still work unattended. Purpose keys are actually initialized and distinct. API tests alone refusing a method are insufficient.
