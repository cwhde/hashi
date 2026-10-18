# AUD-010: Scheduled scripts do not receive configured environment variables

Priority: P2 — missing script behavior

Spec reference: implementation spec §23 lists environment variables as script fields and requires cron/systemd scheduling as well as manual execution.

## Finding

Manual script execution calls `ResolveEnvironmentVariablesAsync` and passes the resulting values to `BuildRunCommand`. The scheduled execution paths do not: `RenderCron` emits a bare `bash` command, and `RenderSystemdService` emits only `ExecStart=/bin/bash ...`. Neither output references an environment file or otherwise injects the script's configured variables. Scheduled scripts that rely on configured variables therefore behave differently from manual runs and fail or operate on unintended defaults; secret variables are also absent.

## Evidence

- `src/Hashi.Infrastructure/Platform/ScriptExecutionService.cs`: manual execution resolves variables before calling `ExecuteRemoteAsync`/`BuildRunCommand`.
- The same file's `RenderCron` builds schedule lines from timeout and script path only.
- The same file's `RenderSystemdService` sets `ExecStart` and timeout but does not add configured environment.
- `SyncHostScriptsAsync` writes the manifest, script, cron file, and systemd units but does not materialize an appropriately protected environment file for scheduled use.

## Expected outcome

Both scheduled backends must run with the configured environment. Secret values must remain root-readable and must not appear in cron files, logs, manifests, or returned API data.

## Acceptance criteria

- Scheduled cron and systemd runs receive the same configured variables as manual runs.
- Secret values are stored remotely with restrictive permissions and are excluded from rendered logs and manifests.
- Removing or rotating a variable updates/removes the scheduled environment safely.
