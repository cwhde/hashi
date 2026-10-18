# AUD-042: Settings page duplicates the theme control ID

Priority: P3 — duplicate form IDs break label targeting and accessibility

Category: Admin UI correctness / accessibility

Spec reference: `docs/implementation-spec/hashi-v2-implementation-spec.md` §21 requires a coherent admin UI; the UI design foundation addendum §21 requires accessible form labeling.

## Finding

The General settings section renders two theme `<select>` elements with the same `id="settings-theme"`. Both labels also use `for="settings-theme"`. This violates the document's unique-ID requirement and makes label activation and accessible control naming ambiguous.

## Evidence

- `web/src/routes/(admin)/settings/+page.svelte:529` and `:566` each render a select with `id="settings-theme"`.
- Both corresponding labels reference `for="settings-theme"`.

## Expected outcome

Render one theme selector, or give every distinct control a unique ID and an associated label.

## Acceptance criteria

- The General settings form has one theme selector unless a second distinct control is intentionally designed.
- Every document ID is unique and every form control has an unambiguous label association.
