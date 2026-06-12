# H-089: Script Page No Diff View or Target Host List

**Priority:** Medium
**Conflict Type:** missing_implementation
**Spec Reference:** §23 (UI shows target hosts and diff before applying)

**Status:** Fixed
**Branch:** audit-series-h

## Description

The scripts page shows name, cron, last run, and enabled — but no diff view for script body changes and no target hosts list. The spec requires showing target hosts and diff before applying script changes.

## Evidence

- Script editor has no diff view component
- Script detail page does not display target hosts
- Apply action does not show confirmation with diff and targets

## Expected Outcome

Script editor should show a diff of changes vs the currently deployed version. Script detail should show a list of target hosts with connection names. Apply should require confirmation showing both diff and targets.

## Fix Guidance

1. Add a diff view component to the script editor showing changes vs the currently deployed version.
2. Show target host list with connection names.
3. Add confirmation step before apply that displays diff and targets.

## Acceptance Criteria

- [x] Script editor shows diff of changes vs deployed version
- [x] Script detail shows list of target hosts
- [x] Apply requires confirmation showing diff and targets
