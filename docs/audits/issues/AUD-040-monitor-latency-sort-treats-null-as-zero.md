# AUD-040: Monitor latency sort treats missing values as zero

Priority: P3 — endpoints without a check result sort ahead of measured endpoints

Category: Admin UI correctness

Spec reference: `docs/implementation-spec/hashi-v2-implementation-spec.md` §18.5 includes sorting monitors by latency.

## Finding

`nullableNumber` converts its input with `Number(value)`. JavaScript converts `null` to `0`, so a monitor with no latency measurement is sorted as the fastest endpoint instead of being placed after measured values. The status page's `formatLatency` helper makes the same conversion, so a missing measurement is also displayed as `0 ms`.

## Evidence

- `web/src/routes/(admin)/status/+page.svelte`: latency sort calls `nullableNumber(a.lastLatencyMs)`; the helper only treats non-finite conversions as missing, and `Number(null)` is finite zero.
- The same page's `formatLatency()` applies `Number(value)` without a null check and formats a missing last latency as `0 ms`.

## Expected outcome

Treat `null`, `undefined`, and nonnumeric latency values as missing and sort them consistently after measured values.

## Acceptance criteria

- Endpoints with null latency sort after endpoints with a numeric latency.
- Endpoints with null latency display an unavailable marker rather than `0 ms`.
- Numeric latency values retain ascending order.
