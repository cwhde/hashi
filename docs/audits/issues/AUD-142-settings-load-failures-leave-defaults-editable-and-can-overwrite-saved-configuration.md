# AUD-142: Settings load failures leave defaults editable and can overwrite saved configuration

Priority: P2

Category: Frontend reliability / settings

Status: open; confirmed by manual source trace.

## Finding

The settings page catches initial load errors as offline-dev cases without showing a load failure or disabling Save. Form defaults remain editable. A transient/authentication/API failure followed by Save can therefore replace real settings with default or blank values the administrator never loaded. Similar independent panel loads suppress errors, so the page can combine stale/default state with successfully loaded sections.

## Evidence

web/src/routes/(admin)/settings/+page.svelte:116-222 and save handlers. General settings form defaults include blank domains/internal URL and an assumed sync interval.

## Expected outcome and acceptance criteria

Track load/error state per section, show the error and require a successful load before enabling edits/save, or use explicit patch semantics with concurrency versions. A failed initial GET must never permit accidental replacement by defaults.
