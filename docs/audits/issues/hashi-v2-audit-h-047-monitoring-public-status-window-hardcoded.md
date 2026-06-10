# H-047: MonitoringService PublicStatusAsync Hardcodes 1-Hour Window Regardless of User Request

**Priority:** Low
**Conflict Type:** bad_implementation
**Spec Reference:** Main Spec §18.3, §18.5, §24

**Status:** Fixed
**Branch:** audit-series-h

## Description

`MonitoringService.PublicStatusAsync` in `src/Hashi.Infrastructure/Platform/MonitoringService.cs` hardcodes a 1-hour window for public status data:

```csharp
// MonitoringService.cs — PublicStatusAsync
// Uses a hardcoded 1-hour window regardless of requested time range
var since = DateTimeOffset.UtcNow.AddHours(-1);
```

The spec §18.5 requires the status landing page to show "last 60 minutes color strip" as part of the default view, but it should also support "Last 1 hour", "Last 24 hours", "Last 7 days", and "Last 30 days" as described in §18.3 required views. The hardcoded 1-hour window prevents users from viewing longer time ranges on the public status page.

## Evidence

```csharp
// MonitoringService.cs — PublicStatusAsync hardcodes since = now - 1 hour
```

The `MonitorRollupEntity` table already stores data at multiple resolutions (1m, 5m, 1h), so there's no technical reason to limit the public status page to 1 hour.

## Expected Outcome

The public status API should accept a configurable time range parameter and use the appropriate rollup resolution for longer windows. The compact 60-minute strip should remain the default but the API should support longer ranges.

## Fix Guidance

1. Make the time window configurable on the public status API endpoint.
2. Use `MonitorRollupEntity` with the appropriate `IntervalMinutes` for longer time ranges.
3. Default to 1 hour for the compact view but allow 24h, 7d, 30d.
4. Apply the settings from §24 (Monitoring > Retention) for maximum lookback.

## Acceptance Criteria

- [ ] Public status API accepts a configurable time range parameter
- [ ] Last 60 minutes strip remains the default
- [ ] 24h, 7d, and 30d views return data from monitor rollup tables
- [ ] Time range respects configured data retention limits
