# AUD-093: Script diff recomputes a quadratic matrix on each edit

Priority: P2

Category: Performance / scripts editor

Status: open; confirmed by manual source tracing.

## Finding

computeDiff allocates a (original lines+1) by (modified lines+1) matrix and traverses it for each body edit through a derived value. It additionally prepends each diff row with Array.unshift. Large but valid maintenance scripts can exhaust browser memory or freeze editing before the confirmation dialog; no line/size bound or worker exists.

## Evidence

web/src/routes/(admin)/scripts/+page.svelte:46,207-246

This is lower priority than installation/security/missing-feature blockers.

## Expected outcome

Use a bounded or linear-space diff implementation, debounce/worker expensive computation, and present a fallback for large files.

## Acceptance criteria

Large scripts remain editable and confirmable without quadratic memory growth; diff remains accurate for insertions/deletions and repeated lines.
