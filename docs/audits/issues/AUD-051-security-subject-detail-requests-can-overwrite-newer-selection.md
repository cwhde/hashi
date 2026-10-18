# AUD-051: Security subject detail requests can overwrite a newer selection

Priority: P3 — quickly selecting multiple subjects can display stale details under the wrong selection

Category: UI race / security dashboard correctness

Spec reference: implementation spec §20 requires an operational security dashboard with accurate per-subject detail and effective-decision views.

## Finding

`selectSubject()` starts four parallel requests and assigns their results unconditionally. If a user selects subject B while subject A's requests are still running, A's later response can replace the detail, effective decision, timeline, and buckets after B has been selected. The selected subject ID remains B, leaving a mismatched security view.

## Evidence

- `web/src/routes/(admin)/security/+page.svelte`: `selectSubject(id)` updates `selectedSubjectId`, starts detail/decision/events/bucket requests, then writes results without checking whether `id` still equals the selected subject.

## Expected outcome

Only commit results from the latest subject selection. Cancel older requests where possible or use a monotonically increasing request key.

## Acceptance criteria

- Selecting a new subject while earlier requests are pending keeps the displayed detail aligned with the latest selection.
- A failed or slower earlier selection cannot overwrite the newer detail or clear its loading state.
- Subject search auto-selection follows the same latest-request rule.
