# AUD-043: Monitor status strip marks a tied bucket as healthy

Priority: P2 — mixed or failed probe buckets can appear green

Category: Monitoring status correctness

Spec reference: `docs/implementation-spec/hashi-v2-implementation-spec.md` §18.5 requires a last-60-minute status strip that reflects endpoint health.

## Finding

The status page reduces each one-minute rollup to a boolean using `upCount >= downCount`. A bucket with equal up and down samples is therefore rendered green as healthy. For example, two successful probes and two failed or degraded probes in a minute are displayed as “up.” No mixed/degraded visual state is represented by the boolean strip component.

## Evidence

- `web/src/routes/(admin)/status/+page.svelte`: `stripFor` maps rollups with `Number(r.upCount) >= Number(r.downCount)`.
- `web/src/lib/components/monitoring/MonitorStatusStrip.svelte`: each bucket is rendered only as up/green or empty/gray; all non-up states are otherwise reduced to the same boolean upstream.

## Expected outcome

Represent mixed or degraded samples explicitly, or use a conservative aggregation rule that never labels an equal up/down bucket healthy. Keep empty buckets visually distinct from checked and unhealthy buckets.

## Acceptance criteria

- Equal up/down counts are not rendered as healthy.
- A bucket containing failures or degraded samples has an accessible, visually distinct state appropriate to the chosen aggregation rule.
- Empty buckets remain distinct from checked-up buckets.
