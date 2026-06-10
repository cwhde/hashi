# H-098: AdGuard Duplicate Rewrite Rows Not Cleaned Up During Sync

**Priority:** Low
**Conflict Type:** missing_implementation
**Spec Reference:** §16 (Clean up duplicate Hashi-managed rewrites on sync)

**Status:** Fixed
**Branch:** audit-series-h

## Description

No explicit deduplication of multiple local `AdGuardRewriteEntity` entries for the same domain exists. While upsert operations use `SingleOrDefaultAsync` (which would throw on duplicates), there's no cleanup step in `PlanSyncAsync` that detects/removes duplicate local rows.

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

- [ ] Duplicate AdGuard rewrite rows are cleaned up during sync
- [ ] Only one rewrite per domain exists after cleanup
- [ ] Cleanup preserves the most recent or most specific rewrite
