# B07 - ACME EAB secret can be stored plaintext during setup

Priority: High

Spec conflicts: non-negotiable rule 5, section 7.4, and section 8. ACME EAB secrets must be encrypted at rest and must not be stored as plaintext setup state.

## Problem

When certificate provider settings are saved while the vault is locked, Hashi serializes the ACME EAB key ID and HMAC into `SetupStateEntity.PendingAcmeEabJson`. That field is a normal database string. A later vault unlock migrates it into a secret record, but until then a database dump contains the plaintext EAB credential.

## Evidence

- `src/Hashi.Infrastructure/Persistence/Entities/CoreEntities.cs:58` defines `PendingAcmeEabJson` as a nullable string on setup state.
- `src/Hashi.Infrastructure/Platform/CertificateSetupService.cs:88` serializes the EAB key ID and HMAC into `eabPayload`.
- `src/Hashi.Infrastructure/Platform/CertificateSetupService.cs:89-96` stores the payload as an encrypted secret only when the vault is unlocked.
- `src/Hashi.Infrastructure/Platform/CertificateSetupService.cs:114` writes the same payload into `setup.PendingAcmeEabJson` when the vault is not unlocked.
- `src/Hashi.Infrastructure/Platform/CertificateSetupService.cs:122-150` migrates the pending payload only after a later unlocked vault state.
- `src/Hashi.Infrastructure/Platform/CertificateSetupService.cs:184-186` can read the pending plaintext payload back for Traefik options.

## Expected outcome

ACME EAB credentials must never be persisted as plaintext. Setup should either require the vault before accepting EAB secrets or encrypt the pending value with a setup-safe key that is not stored alongside the ciphertext.

## Fix guidance

Move certificate secret collection after vault setup, or make the save endpoint reject EAB credentials until the vault is unlocked. If resumability is required before vault setup, store only non-secret certificate settings and ask for EAB credentials again after vault creation.

## Acceptance criteria

- No database column stores ACME EAB HMAC plaintext.
- Saving certificate settings while the vault is locked does not persist EAB credentials.
- Saving certificate settings after vault unlock stores EAB credentials in `SecretRecordEntity`.
- Tests assert `PendingAcmeEabJson` is removed or never populated with secret material.
