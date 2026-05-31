# B06 - Service-sync secrets omit unattended runtime credentials

Priority: Critical

Spec conflicts: section 8 and Phase 16. Background provider jobs must continue through the service-sync vault without an active browser session, while only required runtime secrets are decryptable.

## Problem

Several secrets needed by unattended or public runtime workflows are stored with the default `serviceSyncEligible = false`. `SecretRecordService` only creates a service-wrapped DEK when that flag is true. When no admin vault session is unlocked, these secrets cannot be decrypted by background jobs or public edge-login flows.

This affects SSH credentials used by Traefik sync, firewall apply, and access-log ingest; notification provider tokens used by background alerts; and OIDC client secrets used by Edge SSO callbacks.

## Evidence

- `src/Hashi.Infrastructure/Auth/SecretRecordService.cs:20` defaults `serviceSyncEligible` to `false`.
- `src/Hashi.Infrastructure/Auth/SecretRecordService.cs:33-44` creates `ServiceWrappedDekBlob` only when `serviceSyncEligible` is true.
- `src/Hashi.Infrastructure/Auth/SecretRecordService.cs:82-90` refuses service-sync decrypts for non-eligible secrets.
- `src/Hashi.Infrastructure/Connections/SshConnectionService.cs:45-49` stores SSH credentials without `serviceSyncEligible: true`.
- `src/Hashi.Infrastructure/Notifications/NotificationDispatcher.cs:352-356` stores notification tokens without `serviceSyncEligible: true`.
- `src/Hashi.Infrastructure/Platform/OidcProviderAdminService.cs:28-32` and `src/Hashi.Infrastructure/Platform/OidcProviderAdminService.cs:86-90` store OIDC client secrets without `serviceSyncEligible: true`.
- `src/Hashi.Infrastructure/Platform/TraefikSyncService.cs:172`, `src/Hashi.Infrastructure/Platform/FirewallApplyService.cs:47`, and `src/Hashi.Infrastructure/Platform/AccessLogIngestWorker.cs:51` need SSH credentials during runtime work.
- `src/Hashi.Infrastructure/Notifications/NotificationDispatcher.cs:380-381` and `src/Hashi.Infrastructure/Platform/OidcEdgeAuthService.cs:152-153` fail if their secrets cannot decrypt outside an admin session.

## Expected outcome

Secrets required for background reconciliation and public runtime flows must be decryptable through the service-sync vault or an equivalent constrained runtime mechanism. Secrets not needed by runtime jobs must remain admin-session-only.

## Fix guidance

Classify secret purposes by runtime need. Store Traefik/firewall SSH credentials, notification tokens, and Edge SSO client secrets with an explicit service-sync/runtime eligibility policy where required. Keep generic script-only SSH credentials out of service-sync unless a scheduled script explicitly needs them and the user approves that risk.

## Acceptance criteria

- Traefik sync can apply without an active admin browser session when service-sync vault is ready.
- Firewall apply and blocklist sync can run without an active admin browser session when service-sync vault is ready.
- Background notifications can decrypt provider tokens through the runtime path.
- Edge SSO callback does not require the admin vault session to be unlocked.
- Secret eligibility tests prove unrelated admin-only secrets are not service-sync decryptable.
