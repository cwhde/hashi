# AUD-050: Concurrent SSH atomic writes share a fixed temporary path

Priority: P2 — overlapping writes to one remote path can corrupt or fail a deployment

Category: Deployment reliability / concurrency

Spec reference: implementation spec §10 requires generated configuration to be validated and safely applied atomically.

## Finding

`SshRemoteExecutor.WriteAtomicCore` stages every write at the deterministic path `<remotePath>.hashi.tmp`. Two operations writing the same destination concurrently open and overwrite the same staging file, then independently hash and rename it. One operation can observe the other's bytes, fail its hash check, delete the shared temporary file, or rename content that belongs to the other operation. This can turn otherwise valid concurrent syncs into failed or wrong deployments.

## Evidence

- `src/Hashi.Infrastructure/Ssh/SshRemoteExecutor.cs`: `tempPath` is always `$"{remotePath}.hashi.tmp"`; no per-operation suffix or serialization is used in `WriteAtomicCore`.
- The method verifies the temporary file's content hash and then performs `mv -f`, but those operations are not protected as a unit from another writer.
- Multiple background/manual sync paths can target the same managed remote files; [AUD-034](AUD-034-subsystem-applies-bypass-global-lock.md) separately records that direct subsystem apply routes bypass the global apply lock.

## Expected outcome

Give each write a unique temporary path and ensure validation and rename cannot race with cleanup from another write. Preserve same-directory staging so the final rename remains atomic.

## Acceptance criteria

- Concurrent writes to the same destination use distinct staging files.
- Each write validates and publishes only its own staged bytes.
- Failed/cancelled writes clean up only their own temporary file.
- The destination is never replaced with bytes from a different write operation.
