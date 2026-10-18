# AUD-045: Fresh setup requires an unlocked vault before the wizard creates it

Priority: P1 — the documented first-run wizard cannot advance through certificate setup

Category: Setup flow / installability

Spec reference: `docs/implementation-spec/hashi-v2-implementation-spec.md` §7 defines the ordered first-run setup flow and §8 requires ACME EAB credentials to be stored in the vault.

## Finding

The specification itself orders certificate setup (§7.4) before passkey/vault setup (§7.8), while requiring ACME secrets to be stored encrypted (§7.4 and §8). The implementation avoids storing EAB material in plaintext and `CertificateSetupService.SaveAsync` returns failure unless `VaultSessionState.IsUnlocked`. On a fresh installation, no vault can be unlocked at the certificate step because the wizard has not yet reached the step that creates the recovery-key wrap and registers the passkey. The certificate step UI only advances when that save succeeds, and it has no defer/skip action. A user following the provided wizard cannot reach vault setup through the normal UI.

The step-completion API can be called out of order, but the wizard provides no navigation control for skipping ahead; requiring a user to manually invoke an internal API is not a working setup flow.

## Evidence

- `web/src/lib/setup/steps.ts`: `certificate-provider` precedes `passkey-and-vault`.
- `src/Hashi.Infrastructure/Platform/CertificateSetupService.cs`: `SaveAsync` saves nonsecret settings, then returns failure when the vault is locked before storing EAB credentials.
- `web/src/lib/components/setup/steps/CertificateProviderStep.svelte`: `saveAndContinue` calls `oncomplete()` only when `result.saved` is true; there is no defer/skip action.
- `src/Hashi.Infrastructure/Auth/SetupCompletionService.cs`: vault configuration and unlock are required later for setup completion.

## Expected outcome

Resolve the specification's sequencing conflict and provide a secure first-run path. Possible designs include moving vault setup earlier, using a separate securely protected bootstrap envelope for EAB material, or allowing ACME configuration to be deferred until after vault setup. The normal wizard must provide a supported path through all required steps without plaintext secret persistence.

## Acceptance criteria

- A clean installation can complete every required wizard step using only visible UI actions.
- EAB credentials are never stored before the vault protects them.
- Deferring ACME configuration is explicit and does not falsely mark it configured.
- Setup completion reports clearly whether HTTPS/ACME prerequisites are configured or intentionally deferred.

## Expanded setup trace

The circular dependency starts at DNS provider creation, before the certificate step. DnsConnectionService.CreateHetznerConnectionAsync calls SecretRecordService.StoreAsync, whose first guard requires an unlocked admin vault. SshConnectionService.CreateAsync has the same requirement. Both backend SetupStep and frontend setup/steps.ts put DNS/certificate/SSH before passkey-and-vault. Reordering only the certificate page does not fix the complete setup dependency graph.
