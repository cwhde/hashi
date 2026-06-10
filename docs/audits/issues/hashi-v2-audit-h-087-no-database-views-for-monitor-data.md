# H-087: No Database Views for Monitor Data

**Priority:** Low
**Conflict Type:** missing_implementation
**Spec Reference:** §18.4 (Required views: last 60 minutes bar, last 1h/24h/7d/30d latency and uptime, event timeline)

**Status:** Fixed
**Branch:** audit-series-h

## Description

No database views for monitor data are implemented. The spec mentions "required views" for efficient querying of monitor rollup data at different time windows.

## Evidence

- No PostgreSQL views exist for monitor data aggregations
- Monitor queries likely hit raw tables without pre-aggregated views

## Expected Outcome

PostgreSQL views should exist for the required monitor data aggregations: last 60 minutes bar, last 1h/24h/7d/30d latency and uptime, and event timeline. Views should be created via migrations and use partition pruning for efficient querying.

## Fix Guidance

1. Add PostgreSQL views for the required monitor data aggregations.
2. Create these views via EF Core migrations.
3. Ensure views use partition pruning for efficient querying.
4. Update monitor API queries to use these views.

## Acceptance Criteria

- [ ] Database views exist for 60-min bar, 1h/24h/7d/30d latency/uptime, event timeline
- [ ] Views use partition pruning for efficient querying
- [ ] Monitor API queries use these views
