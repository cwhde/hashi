# H-098: AdGuard Duplicate Rewrite Rows Not Cleaned Up During Sync

**Priority:** Low
**Conflict Type:** missing_implementation
**Spec Reference:** §16 (Clean up duplicate Hashi-managed rewrites on sync)

**Status:** Fixed
**Branch:** audit-series-h

## Description

The local table has a unique `(ConnectionId, Domain)` index, so duplicate local rows are prevented by the database. The real operational gap was duplicate remote AdGuard rows for a Hashi-managed domain. Apply now removes remote extras, preserves a row matching desired state when present, and audits the cleanup.

## Evidence

- No deduplication logic in AdGuard `PlanSyncAsync`
- `SingleOrDefaultAsync` would throw on duplicate entries rather than clean them up
- Duplicate local rows could accumulate from race conditions or bugs

## Expected Outcome

Duplicate AdGuard rewrite rows should be cleaned up during sync. Only one rewrite per domain should exist after cleanup. Cleanup should preserve the most recent or most specific rewrite.

## Fix Guidance

1. Add a deduplication step in AdGuard `PlanSyncAsync` that detects duplicate domains in local rewrites.
2. Remove extras, keeping the most recent.
3. Log deduplication events for auditability.

## Acceptance Criteria

- [x] Duplicate AdGuard rewrite rows are cleaned up during sync
- [x] Only one remote rewrite per Hashi-managed domain exists after cleanup
- [x] Cleanup preserves the rewrite matching Hashi desired state when present
