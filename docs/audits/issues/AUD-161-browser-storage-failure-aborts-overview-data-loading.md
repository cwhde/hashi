# AUD-161: Browser storage failure aborts overview data loading

Priority: P3

Category: Frontend reliability / stored preferences

Status: open; confirmed by manual source trace.

## Finding

saveWidgetPrefs calls localStorage.setItem without handling quota/security failures. loadDashboardWidgetPrefs invokes it during initial server preference loading, before AdminOverviewView assigns the already fetched dashboard data. A blocked/quota-exhausted storage environment therefore aborts the load path even when the API returned valid dashboard data; the enclosing catch silently leaves default widgets and counts. The order parser does check Array.isArray, so a malformed order value alone is not evidence of a crash. Full preference schema validation and duplicate normalization would still improve predictable behavior.

## Evidence

web/src/lib/overview/widgets.ts:66-90 and normalization helpers; web/src/lib/components/admin/AdminOverviewView.svelte:26-33.

## Expected outcome and acceptance criteria

Validate the full preference schema, canonicalize unique known widget IDs and treat cache write failure as nonfatal. Corrupt preferences or disabled browser storage must fall back without blocking dashboard data.
