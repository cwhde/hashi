# AUD-078: Sliding rollup windows overwrite complete historical buckets with partial data

Priority: P2

Category: Monitoring / historical integrity

Status: open; confirmed by manual source tracing.

## Finding

RollupIntervalAsync filters samples from the exact moving cutoff then upserts all buckets touched by those samples. As cutoff moves through the oldest1/5/60-minute bucket, that bucket is recomputed using only its remaining suffix and overwrites previously complete counts/latency. Once it leaves the window, the corrupted partial bucket persists for months/years. The check worker and separate rollup worker also run this operation concurrently.

## Evidence

Paths below are relative to src/Hashi.Infrastructure/:

Platform/MonitorRollupService.cs:15-17,28-65; Platform/MonitorCheckWorker.cs:129-130; Platform/MonitorRollupWorker.cs:25

## Expected outcome

Align source cutoffs to whole buckets or finalize closed buckets once; coordinate concurrent upserts.

## Acceptance criteria

Historical complete bucket counts/latency remain stable as the recomputation window advances, including exact boundaries and concurrent workers.

No live service configuration or firewall was changed to establish this finding.
