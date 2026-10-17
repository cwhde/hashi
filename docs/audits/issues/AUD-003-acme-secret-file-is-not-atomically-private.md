# AUD-003 - ACME credential file can be published before restrictive permissions are applied

Priority: Medium (P2)

Category: Security / secret file permissions

Specification reference: `docs/implementation-spec/hashi-v2-implementation-spec.md`, sections 3 and 6 (security invariants and encrypted secret storage); section 10 (ACME EAB secrets).

## Problem

Traefik apply writes the ACME DNS credential file through the generic atomic SFTP writer, which creates and renames the file before a later SSH command applies mode `0600`. If the apply process or SSH connection fails after the rename but before `chmod 600`, the secret file remains with the SFTP server's default creation permissions (commonly `0644`). During the interval before chmod, local users may also read it. This is a DNS provider credential exposure on the managed host.

## Evidence

- `TraefikSyncService.ApplyForConnectionInternalAsync` writes `/etc/hashi/traefik/acme.env` with `WriteAsync`, then calls `ConfigureAcmeEnvironmentAsync`.
- `ConfigureAcmeEnvironmentAsync` applies `chmod 600` only after the write has completed.
- `SshRemoteExecutor.WriteAtomicCore` opens the temporary file with `SftpClient.OpenWrite` and atomically renames it without setting restrictive file attributes.
- A process/connection failure between those operations leaves the final path with its default mode.

## Expected outcome

Secret-bearing remote files are created with restrictive permissions before they become visible at the final path. An interrupted write must not leave a readable secret file behind.

## Acceptance criteria

- Extend the remote atomic writer to support an explicit mode/owner policy, or use a secret-specific remote write that sets mode `0600` on the temporary file before rename.
- Ensure the final ACME file remains mode `0600` after replace and after rollback/recovery.
- Add tests that verify the command/order and failure cleanup paths; where possible, exercise file modes on a disposable SSH target.
- Keep ordinary non-secret configuration permissions compatible with Traefik operation.
