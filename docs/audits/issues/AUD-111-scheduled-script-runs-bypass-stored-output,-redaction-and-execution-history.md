# AUD-111: Scheduled script runs bypass stored output, redaction and execution history

Priority: P1

Category: Missing feature / scheduled script observability

Status: open; confirmed by manual source trace.

## Finding

The scheduler deploys autonomous systemd/cron commands that write directly to /var/log/hashi/scripts/*.log. Only the manual ExecuteRemoteAsync path calls CompleteRunAsync to capture/redact output and write ScriptRun/ScriptOutput rows. There is no ingestion of scheduled logs or their exit status; secret material printed by a scheduled script remains unredacted in host logs, and the UI has no actual scheduled-run history. SyncAllEnabledScriptsAsync also swallows each host/target exception while ScriptCronHostedService marks the overall deployment job successful. Scheduled environment omission is separately AUD-010.

## Evidence

src/Hashi.Infrastructure/Platform/ScriptExecutionService.cs:195-235,536-574,830-859,1019-1034; ScriptCronHostedService.cs:18-27. Main specification23 requires output captured, redacted and stored for script execution.

## Expected outcome and acceptance criteria

Implement a bounded scheduled execution/result collection path with secret redaction, per-host outcome and retention. Preserve failed deployment details and report partial failure. Scheduled successes/failures must appear as real runs without leaking secret values into persisted output.
