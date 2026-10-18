# AUD-012: Monitor rollups use overlapping in-memory scans and per-bucket queries

Priority: P3 — performance / scalability

Spec reference: implementation spec §18.4 requires minute/five-minute/hour rollups and partitioned raw samples to keep monitoring storage efficient.

## Finding

Each rollup pass loads all raw samples for three overlapping windows (3 hours, 1 day, and 7 days) into application memory, groups them client-side, then performs a separate database lookup for every monitor/bucket/interval group before saving. This repeats work over the same sample rows and creates an N+1 query pattern that grows with monitor count and retention volume.

## Evidence

- `src/Hashi.Infrastructure/Platform/MonitorRollupService.cs`: `RollupRecentAsync` invokes three interval passes with overlapping windows.
- `RollupIntervalAsync` calls `ToListAsync` on all samples in the time range, groups them in process, and issues `SingleOrDefaultAsync` once per group.
- `MonitorRollupWorker` repeats the process every five minutes, and `MonitorCheckWorker` also invokes `RollupRecentAsync` after a check cycle.

## Expected outcome

Rollup work should aggregate in PostgreSQL and update only changed/recent buckets, without repeatedly loading the full raw sample windows or querying one rollup row at a time.

## Acceptance criteria

- Database-side grouped aggregation and batch upsert replace client-side aggregation and per-bucket lookups.
- The monitor worker does not duplicate the same full rollup pass on every check cycle and every five-minute timer tick.
- Query count and memory use remain bounded as monitor history grows.
