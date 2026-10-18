# AUD-160: Public health views silently show empty or stale data after request failure

Priority: P2

Category: Frontend reliability / public health

Status: open; confirmed by manual source trace.

## Finding

PublicDashboardView and PublicStatusView fetch once on initialization with no polling/invalidation. Errors become null/empty arrays, displayed as no apps/zero hosts or no endpoints rather than telemetry unavailable. A long-lived status page never reflects subsequent outages/recovery. Some status presentation uses a green badge even for Offline or zero available endpoints. This can mislead a visitor about service health independently of the backend aggregate/tie defects.

## Evidence

web/src/lib/components/public/PublicDashboardView.svelte:15-33 and status display; PublicStatusView.svelte fetch catch and summary; main specification public status/dashboard operational behavior.

## Expected outcome and acceptance criteria

Represent loading, stale, failed and genuinely empty states distinctly; refresh live health with bounded lifecycle-aware polling/events and render canonical health colors. A request failure must not imply an empty healthy installation.
