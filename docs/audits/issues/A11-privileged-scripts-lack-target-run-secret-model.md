# A11 - Privileged scripts lack the specified target, secret, output, and deployment model

Priority: High

Spec conflicts: non-negotiable rules 5 and 18; scripts section 22.

## Problem

Scripts are privileged root-capable operations, but the current model stores script bodies and last output directly in the `scripts` table, has no target-host table, has no per-run/output table, and has no encrypted environment variable model. Script output is stored and returned without redaction.

Deployment also writes the remote script before creating `/opt/hashi/scripts`, so fresh hosts can fail before the later `mkdir -p`. The script is only made executable with `chmod +x`; there is no ownership or mode hardening.

The direct run endpoint can accept ad-hoc SSH credentials and execute the script against an arbitrary host, which is much broader than the spec's "target hosts, default all Linux firewall hosts" model.

## Evidence

- `src/Hashi.Infrastructure/Persistence/Entities/ExtendedPlatformEntities.cs:393-412` defines a single `ScriptEntity` with `Body` and `LastRunOutput`.
- `src/Hashi.Contracts/Api/PlatformContracts.cs:494-566` has no encrypted env var, target-host, run, or output model.
- `src/Hashi.Infrastructure/Platform/ScriptExecutionService.cs:163-195` writes `/opt/hashi/scripts/{id}.sh` before running `mkdir -p /opt/hashi/scripts`.
- `src/Hashi.Infrastructure/Platform/ScriptExecutionService.cs:195` only runs `chmod +x`.
- `src/Hashi.Infrastructure/Platform/ScriptExecutionService.cs:244-248` stores raw output in `LastRunOutput` and audits only success/failure.
- `src/Hashi.Api/Features/Platform/PlatformEndpoints.cs:605-614` allows direct run with ad-hoc request host/credentials.

## Expected outcome

Scripts should have target hosts, encrypted secret environment variables, per-run records, redacted captured output, and passkey reauthentication before saving or running. Deployment should create directories before atomic writes and enforce root-owned non-world-writable permissions.

## Fix guidance

Add `host_script_targets`, `host_script_runs`, and output records, or equivalent tables. Add encrypted env var storage through secret records. Create remote directories before write. Harden file owner/mode. Restrict direct run to configured target hosts unless a separate audited emergency flow is designed.

## Acceptance criteria

- Creating/updating/running scripts requires recent passkey reauth.
- Script secrets are encrypted and never returned or logged.
- Each run has a durable record with redacted output and target host status.
- Fresh-host deployment creates required directories before atomic write.
