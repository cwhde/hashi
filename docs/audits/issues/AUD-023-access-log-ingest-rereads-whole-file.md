# AUD-023: Access log ingestion rereads whole files and misses some rotations

Priority: P2 — operational performance / ingestion reliability

Category: Inefficiency and data-loss edge case

Spec reference: implementation addendum §2 and implementation spec §24 require reliable access-log ingestion with request correlation and bounded operational work.

## Finding

On every one-minute run, `AccessLogIngestWorker` reads the complete configured remote access log over SFTP into memory, then slices the byte array at the saved offset. Transfer volume and memory therefore grow with total log size rather than newly appended data. The persisted cursor tracks only a byte offset, not the remote file identity or rotation generation. If rotation replaces the file with a new file whose length is greater than or equal to the previous offset, the worker resumes at that offset and permanently skips the new file's leading entries.

There is also a crash window that can duplicate ingested events. Each parsed line is handed to `SecurityIngestionService.IngestAccessLogAsync`, which saves its database changes immediately. The file cursor is saved only after the host's full line loop. If the worker exits after one or more lines have been committed but before that cursor save, the next run reprocesses those lines and increments the corresponding ingestion records again. The ingestion path has no stable per-line idempotency key to suppress that replay.

## Evidence

- `src/Hashi.Infrastructure/Platform/AccessLogIngestWorker.cs`: each host calls `ISshRemoteExecutor.ReadFileAsync` for the entire log path, then applies the stored `ByteOffset` locally.
- The cursor stores only `ByteOffset` and update time; it has no inode/file identity, modification generation, or rotation marker.
- `src/Hashi.Infrastructure/Ssh/SshRemoteExecutor.cs`: remote reads return file contents as a complete byte array.
- `AccessLogIngestWorker` awaits `SecurityIngestionService.IngestAccessLogAsync` for every parsed line before updating/saving the host cursor.
- `src/Hashi.Infrastructure/Platform/SecurityIngestionService.cs`: `IngestAccessLogAsync` persists each line before returning, so a later interruption can leave committed line data ahead of the cursor.

## Expected outcome

Read only newly appended bytes using a bounded remote tail/read operation, and detect truncation or rotation using a stable file identity or an equivalent safe mechanism. Preserve complete-line handling across polling boundaries.

## Acceptance criteria

- Network transfer and memory are bounded by newly appended log data rather than total historical file size.
- Truncation, rename/create rotation, and copy-truncate rotation reset or advance cursors without losing new entries or duplicating old entries unexpectedly.
- Incomplete trailing lines remain buffered/re-read until complete.
- Cursor checkpoints and event ingestion have crash-safe, idempotent semantics: a crash between line ingestion and checkpoint persistence does not double-count replayed lines, and cursor state never advances beyond durably ingested lines.
