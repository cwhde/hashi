# AUD-041: Monitor range changes issue duplicate, unordered loads

Priority: P3 — redundant requests can overwrite data for the currently selected range

Category: Admin UI correctness / inefficiency

Spec reference: UI design foundation addendum §§18 and 20 require trustworthy loading behavior across views and viewport states.

## Finding

The status page starts `load()` from a reactive `$effect` that reads `hours`, and the range selector also calls `load()` in its `onchange` handler. Changing the range therefore starts two overlapping loads. There is no request-generation check or cancellation; if an older request resolves after a newer request, its rollups and events overwrite the data for the newly selected range.

## Evidence

- `web/src/routes/(admin)/status/+page.svelte`: the `$effect` invokes `load()`, whose request parameters read `hours`.
- The `status-hours` select also invokes `load()` from `onchange`.
- `load()` assigns response data unconditionally after `Promise.all` resolves.

## Expected outcome

Use one reactive load trigger and ensure only the latest selected-range request can update view state.

## Acceptance criteria

- Changing the range issues one request set.
- A slow response for an earlier range cannot replace the currently selected range's data.
- Loading/error state reflects the current request rather than an older overlapping request.
