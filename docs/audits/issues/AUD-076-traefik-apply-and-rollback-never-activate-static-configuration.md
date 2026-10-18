# AUD-076: Traefik apply and rollback never activate static configuration

Priority: P1

Category: Traefik / activation and recovery

Status: open; confirmed by manual source tracing.

## Finding

Install prepares/enables a service without starting it. Apply writes static and dynamic files and records success, but never starts/restarts Traefik or verifies running routes. Rollback likewise rewrites files without restart/verification. Static entry-point/ACME changes cannot take effect through file-provider watching alone, and a fresh install can remain stopped while state reports applied. Multi-file writes can also fail halfway with no automatic rollback.

## Evidence

Paths below are relative to src/Hashi.Infrastructure/:

Platform/TraefikSyncService.cs:170-197,326-367,369-406

## Expected outcome

Implement complete validate/stage/activate/health-verify and rollback lifecycle, recording applied state only after successful activation.

## Acceptance criteria

Fresh installation serves a configured route; new static entry points activate; failed activation restores and verifies the last working complete snapshot.

No live service configuration or firewall was changed to establish this finding.
