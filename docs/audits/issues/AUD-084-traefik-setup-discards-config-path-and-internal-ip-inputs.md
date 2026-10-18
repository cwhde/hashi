# AUD-084: Traefik setup discards config path and internal IP inputs

Priority: P1

Category: Missing feature / Traefik setup configuration

Status: open; confirmed by manual source tracing.

## Finding

The setup form collects Internal Traefik IP and Config path and reports them in its success message, but connectionBody sends neither value. The request contract cannot carry them. TraefikSyncService always uses default state paths and ignores credential settings paths. Consequently the user-facing configuration cannot control where Hashi detects/backs up/writes config or how the selected host is linked internally. The setup UI also claims write-permission validation even though the SSH validator only logs in and reads os-release.

## Evidence

web/src/lib/components/setup/steps/TraefikConnectionStep.svelte:31-32,70-94,105-110,223-235; src/Hashi.Contracts/Api/ConnectionContracts.cs:23-33; src/Hashi.Infrastructure/Platform/TraefikSyncService.cs:249-250,550-561; src/Hashi.Infrastructure/Ssh/SshRemoteExecutor.cs:96-114

## Expected outcome

Persist and honor supported configuration paths/internal topology, and report exactly the validation that ran.

## Acceptance criteria

Nondefault configured paths survive reload and are used for detect/backup/stage/apply/rollback; permission validation fails before live writes when access is insufficient.

No live credentials, host scripts or services were changed for this review.
