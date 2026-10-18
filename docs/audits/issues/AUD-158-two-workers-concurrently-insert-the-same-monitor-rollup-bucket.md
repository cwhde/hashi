# AUD-158: Two workers concurrently insert the same monitor rollup bucket

Priority: P2

Category: Reliability / monitor aggregation concurrency

Status: open; confirmed by manual source trace.

## Finding

MonitorCheckWorker calls RollupRecentAsync after its check loop while a separate MonitorRollupWorker calls it every cycle. RollupIntervalAsync reads then inserts each bucket under a unique endpoint/start/interval index without upsert, retry or shared lock. Concurrent first aggregation can fail a whole pass on duplicate insertion, while stale tracked updates can overwrite newer counts. This is independent of AUD-078's deterministic sliding-window partial overwrite.

## Evidence

src/Hashi.Infrastructure/Platform/MonitorCheckWorker.cs:130; MonitorRollupWorker.cs:24; MonitorRollupService.cs:38-70; Persistence/HashiDbContext.cs monitor rollup unique index.

## Expected outcome and acceptance criteria

Have one aggregation owner or atomic idempotent upserts with bounded concurrency handling. Simultaneous aggregation must yield one complete consistent bucket and no aborted unrelated endpoints.
