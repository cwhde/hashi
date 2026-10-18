# AUD-049: DNS record capabilities can be overwritten by a stale request

Priority: P3 — changing zones quickly can leave the record form with the wrong type choices

Category: UI race / DNS record form correctness

Spec reference: implementation spec §7 requires DNS records to use the capabilities of the selected provider connection.

## Finding

The DNS record form starts an asynchronous capability request whenever its selected zone changes. Responses are applied without checking that the zone is still selected. If an earlier zone's request finishes after a later selection, its supported record types replace the current zone's types, and the form may silently change the selected type to one unsupported by the current connection.

## Evidence

- `web/src/routes/(admin)/dns/+page.svelte`: the `$effect` captures `recordForm.zoneId`, invokes `api.getDnsConnectionCapabilities()`, then assigns `supportedTypes` in `.then()` without comparing the captured zone ID to the current selection.
- `saveRecord()` submits the current `recordForm.type` and zone ID together, so stale choices can produce a rejected save or an invalid provider record request.

## Expected outcome

Ignore capability responses that do not belong to the currently selected zone, and reset the selected type only from the active zone's supported types.

## Acceptance criteria

- Each capability result is associated with the zone/connection request that produced it.
- A delayed response for a previously selected zone cannot replace the current zone's type list.
- Saving always validates the selected record type against the selected zone's current capabilities.
