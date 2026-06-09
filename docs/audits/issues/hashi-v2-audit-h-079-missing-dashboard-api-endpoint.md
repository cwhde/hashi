# H-079: Missing Dashboard API Endpoint

**Priority:** Medium
**Conflict Type:** missing_implementation
**Spec Reference:** Main Spec §27

## Description

The spec lists `/api/dashboard/*` as an admin API endpoint group, but no route maps to `/api/dashboard`. There is `/api/settings/dashboard` (widget preferences), `/api/security/dashboard` (security metrics), and `/api/public/apps` (public dashboard), but no dedicated admin dashboard data endpoint that aggregates overview data for the admin dashboard.

## Evidence

- No `DashboardEndpoints` class or route group mapped to `/api/dashboard` exists in the codebase
- The overview page currently makes multiple individual API calls to populate widgets

## Expected Outcome

A `/api/dashboard` endpoint that returns aggregated overview data (system status, resource health, recent incidents, sync state, etc.) in a single response, reducing frontend API call count.

## Fix Guidance

1. Add a `DashboardEndpoints` class with `GET /api/dashboard` returning a `DashboardResponse` with all overview widget data.
2. This can aggregate calls to existing services.

## Acceptance Criteria

- [ ] `GET /api/dashboard` returns a non-404 response with aggregated overview data
- [ ] Response includes all overview widget data in a single payload
- [ ] Reduces number of API calls needed for the overview page
