# AUD-110: Manual script runs ignore configured timeouts beyond thirty seconds

Priority: P2

Category: Wrong implementation / script runtime

Status: open; confirmed by manual source trace.

## Finding

Scripts accept RunTimeoutSeconds up to86400 and BuildRunCommand applies that remote timeout, but all SSH commands are executed through SshConnectionHelper with a hard-coded30second CommandTimeout. A legitimate longer script fails at the transport deadline before its configured runtime expires. Cancelling the client call also does not establish that the remote process stopped, so the run can be reported failed while side effects continue.

## Evidence

src/Hashi.Infrastructure/Platform/ScriptExecutionService.cs:515-533,872-895; src/Hashi.Infrastructure/Ssh/SshRemoteExecutor.cs:69-79,241-258; SshConnectionHelper.cs:35-46.

## Expected outcome and acceptance criteria

Pass an execution-specific deadline through the SSH abstraction, distinguish connect/command/process timeouts and confirm remote termination. A synthetic long-running script must respect its configured timeout and record an accurate terminal status.
