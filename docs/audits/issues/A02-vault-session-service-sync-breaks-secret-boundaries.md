# A02 - Vault session and service-sync design break secret boundaries

Priority: Critical

Spec conflicts: non-negotiable rules 5, 18, 21, and 24; sections 9.2 and 9.3. Admin vault access must be session-bound. Service-sync vault can decrypt only routine reconciliation secrets.

## Problem

The vault session is a singleton process-wide state. Unlocking the vault for one admin effectively unlocks it for every request. Service-sync can also unlock the same admin root key into that singleton, which means unattended background sync can accidentally make admin-only secrets available to interactive API calls.

New secrets are always wrapped for service-sync when the service key is configured, regardless of purpose. That makes service-sync able to decrypt secrets beyond the minimum routine sync set. Secret reveal also does not require reauthentication because it is a `GET` endpoint and the auth middleware only checks reauth for unsafe methods.

## Evidence

- `src/Hashi.Infrastructure/DependencyInjection.cs:33-34` registers `VaultSessionState` and `ServiceSyncVaultState` as singletons.
- `src/Hashi.Infrastructure/Auth/VaultSessionState.cs:6-35` stores one process-wide `AdminRootKey`.
- `src/Hashi.Infrastructure/Auth/VaultService.cs:159-178` decrypts the service-sync wrapped admin root key and calls `session.Unlock(rootKey)`.
- `src/Hashi.Infrastructure/Auth/SecretRecordService.cs:31-35` creates a service-sync wrapped DEK for every stored secret when service-sync is ready.
- `src/Hashi.Infrastructure/Auth/SecretRecordService.cs:80-101` allows service-sync decryption for any secret with a service-wrapped DEK.
- `src/Hashi.Api/Features/Vault/VaultEndpoints.cs:107-118` reveals secrets over `GET`.

## Expected outcome

Admin vault unlock must be bound to the authenticated admin session. Service-sync must have a separate root/wrap model and be limited to secrets explicitly marked as needed for background reconciliation. Service-sync unlock must not set the admin vault session state. Secret reveal must require recent passkey reauthentication and must be audited.

## Fix guidance

Split admin vault and service-sync vault into separate capabilities. Add secret metadata indicating whether a secret is service-sync eligible. Only DNS/provider/AdGuard/Traefik secrets needed for routine sync should be service-wrapped. Store admin unlock state per session, not globally. Remove service-sync calls to `VaultSessionState.Unlock`.

## Acceptance criteria

- Service-sync jobs can decrypt only explicitly service-sync eligible secrets.
- Admin secret reveal fails unless the current admin session has recently reauthenticated.
- Service-sync startup does not make `/api/vault/status` report the admin vault as unlocked for all users.
- Tests verify admin-only secrets cannot be decrypted through `DecryptForServiceSyncAsync`.
