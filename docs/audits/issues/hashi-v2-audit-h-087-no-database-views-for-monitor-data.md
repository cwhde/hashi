# H-087: No Database Views for Monitor Data

**Priority:** Low
**Conflict Type:** missing_implementation
**Spec Reference:** §18.4 (Required views: last 60 minutes bar, last 1h/24h/7d/30d latency and uptime, event timeline)

**Status:** False positive - verified product views
**Branch:** audit-series-h

## Description

The original finding interpreted "required views" as PostgreSQL `CREATE VIEW` objects. In context, the spec lists user-facing time-range views and then immediately defines their UI. Hashi serves those views from partitioned raw samples and retained 1m/5m/1h rollups; separate SQL view objects are not required by the spec.

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

- [x] 60-min bar, 1h/24h/7d/30d latency/uptime, and event timeline product views exist
- [x] Raw sample retention uses partition pruning and longer ranges use retained rollups
- [x] Monitor APIs select the appropriate rollup interval for the requested range
