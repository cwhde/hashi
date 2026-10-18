# AUD-039: Monitor latency summary reports rollup averages as sample min/max/average

Priority: P2 — required latency summary values are mathematically inaccurate

Category: Monitoring analytics / specification compliance

Spec reference: `docs/implementation-spec/hashi-v2-implementation-spec.md` §18.5 requires detail-page response-time min/max/avg.

## Finding

The status page calculates its min, max, and average from each rollup's `AverageLatencyMs`. Those values are averages for time buckets, not the individual probe latencies. Thus the displayed “Min response” and “Max response” are the lowest/highest bucket averages rather than the actual minimum/maximum probe latency. The overall average is an unweighted mean of bucket averages, so buckets with different `SampleCount` receive equal weight. The function also converts nullable values with `Number()` before filtering; `Number(null)` is zero, so empty-latency buckets can fabricate a zero-millisecond result. The persisted rollup contains only average and sample count, so the raw min/max cannot be reconstructed from it.

## Evidence

- `web/src/routes/(admin)/status/+page.svelte`: `calculateStats` maps `averageLatencyMs` to a list and takes `Math.min`, `Math.max`, and an unweighted arithmetic mean.
- The same function applies `Number()` before checking finiteness, which turns null values into zero.
- `src/Hashi.Infrastructure/Platform/MonitorRollupService.cs`: rollups group raw samples and persist `SampleCount` plus `AverageLatencyMs`, but no minimum, maximum, or latency sum.
- `src/Hashi.Contracts/Api/PlatformContracts.cs`: `MonitorRollupResponse` exposes average latency only.

## Expected outcome

Compute summary statistics from raw samples for supported retention windows, or persist sufficient aggregate statistics (minimum, maximum, and total latency) to combine rollups correctly. Label values only according to what the stored aggregation actually represents.

## Acceptance criteria

- Detail min/max equal the min/max of individual probe latencies in the selected period.
- Average is weighted by probe sample count across rollups.
- Empty periods show no value rather than a fabricated statistic.
- Aggregation remains correct across minute, five-minute, and hourly rollups.
