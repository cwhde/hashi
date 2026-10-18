# AUD-152: SSH and SFTP work continues after cancellation and remote reads are unbounded

Priority: P2

Category: Reliability / SSH cancellation

Status: open; confirmed by manual source trace.

## Finding

The SSH executor wraps synchronous operations in Task.Run with a cancellation token. The token can prevent starting work but cannot stop an already running network operation. SFTP transfers have no explicit operation deadline and copy entire remote files into memory. A stalled read/write or oversized log/config file can retain a worker and memory despite caller cancellation, and global sync locks can remain held until it returns. This differs from AUD-110's fixed command deadline truncating legitimate scripts and AUD-023's repeated full log scans.

## Evidence

src/Hashi.Infrastructure/Ssh/SshRemoteExecutor.cs:69-79,140-147 and read operations; SshConnectionHelper command/client timeouts. Cancellation is not wired to transport disposal/abort.

## Expected outcome and acceptance criteria

Use cancellable transport APIs or controlled disposal, configure bounded connect/operation/read deadlines and size limits, and release orchestration locks on cancellation. Independent stalled/oversized remote streams must terminate predictably without mutating after the operation was reported cancelled.
