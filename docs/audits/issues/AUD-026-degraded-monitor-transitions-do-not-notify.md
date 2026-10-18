# AUD-026: Degraded monitor transitions do not produce notifications

Priority: P2 — monitoring alert feature gap

Category: Missing notification behavior

Spec reference: implementation spec §18.6 requires severity thresholds and per-endpoint notification routing; monitor states include `degraded` in §18.1.

## Finding

`MonitorCheckWorker` sets an endpoint to `degraded` when an HTTP check succeeds slowly, but `NotificationRoutingService.RouteMonitorTransitionAsync` only treats a new `down` state as an incident and a transition to `up` from down/degraded as a recovery. A transition into `degraded` is discarded. Consequently, warning-level notification routes cannot alert on a monitor degrading even though the state and configured latency threshold exist.

## Evidence

- `src/Hashi.Infrastructure/Platform/MonitorCheckWorker.cs`: successful HTTP checks above `MonitorDegradedLatencyMs` are assigned status `degraded`.
- `src/Hashi.Infrastructure/Platform/NotificationRoutingService.cs`: `RouteMonitorTransitionAsync` returns unless the new state is `down` or qualifies as recovery; degraded entry is not routed.
- The notification route model supports a `warning` severity threshold.

## Expected outcome

Route transition into degraded status as a warning notification, with cooldown and per-endpoint match behavior applied consistently. Define and document recovery behavior from degraded.

## Acceptance criteria

- A transition from up to degraded can reach warning-level routes.
- Repeated degraded checks do not spam; cooldown is honored.
- A later transition to up can send a recovery when `SendRecovery` is enabled.
- A down transition after degraded uses the critical severity and remains independently routed.
