# AUD-011: SSH connections do not verify server host keys

Priority: P1 — credential and remote execution security

Spec reference: implementation spec §6 requires SSH connections to manage Traefik and firewall hosts using passwords or private keys; the security target requires those credentials and privileged remote operations to be protected against interception.

## Finding

The SSH client factory creates `SshClient` instances without installing a `HostKeyReceived` verifier or loading a known-hosts/fingerprint policy. No host-key fingerprints are present in the connection model or setup flow. As a result, Hashi has no application-level host identity check before sending passwords/private-key authentication and executing privileged commands or transferring configuration. An attacker able to intercept or redirect the connection can impersonate the configured host.

## Evidence

- `src/Hashi.Infrastructure/Ssh/SshConnectionHelper.cs`: both password and private-key factories create `ConnectionInfo` with credentials and return `new SshClient(connectionInfo)` without host-key validation configuration.
- `rg -n "HostKeyReceived|known_hosts|fingerprint" src` finds no active host identity verification implementation.
- All SSH read, write, and command operations in `SshRemoteExecutor` use clients from this helper.

## Expected outcome

Each configured SSH host has a verifiable identity. First connection should display or require confirmation of its fingerprint; subsequent connections must reject changed keys unless the operator explicitly rotates the trusted key.

## Acceptance criteria

- Store host key fingerprints or use a protected known-hosts store per host/port.
- Reject unknown or changed keys by default after enrollment.
- Expose a clear fingerprint confirmation and controlled rotation flow.
- Cover password, private-key, SFTP, and command paths with mismatch rejection.
