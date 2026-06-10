# H-040: ConnectionTargetResolver Falls Back to 127.0.0.1 on Resolution Failure

**Priority:** Medium
**Conflict Type:** bad_implementation
**Spec Reference:** Addendum §9.4, Main Spec §3 (Non-Negotiable #3: never writes provider state when validation fails)

**Status:** Not Started
**Branch:** 

## Description

In `ConnectionTargetResolver` (`src/Hashi.Infrastructure/Platform/ConnectionTargetResolver.cs`), when target resolution fails for a Pulse agent-based connection (stale agent, missing heartbeat, unresolvable IP), the resolver falls back to `127.0.0.1` instead of failing with an error state:

```csharp
// ConnectionTargetResolver.cs ~line 428
var fallbackUri = new Uri($"http://127.0.0.1:{target.Port}/");
```

This "fail-open" behavior means that when a connection's target is unresolvable:
1. The resolver silently returns a localhost URI
2. Downstream services (AdGuard sync, monitoring, etc.) attempt to connect to `127.0.0.1` instead of the intended remote service
3. Connections appear to succeed locally (connecting to nothing or a wrong local service) while the actual upstream is unreachable

The result is confusing behavior: AdGuard rewrites might sync to nowhere, monitoring might report healthy when the actual target is down, and no error is surfaced until a human notices the data is wrong.

## Evidence

```csharp
// ConnectionTargetResolver.cs
// On resolution failure, returns http://127.0.0.1:{port}/ instead of throwing or returning error
```

The spec addendum §9.4 states: "When an agent-bound connection changes resolved IP: 1. Hashi records the new resolved target. 2. Hashi marks dependent connection health as pending. 3. Hashi queues relevant sync/reconcile jobs." This implies the connection health should reflect the actual resolution state, not silently fall back to localhost.

## Expected Outcome

When a connection target cannot be resolved:
1. The target status should be set to `Failed` with a clear error message
2. No fallback URI should be returned — the caller should handle the error explicitly
3. Dependent connection health should be marked as `Degraded` or `Failed`
4. The error should be surfaced in the connection health UI

## Fix Guidance

1. Remove the localhost fallback in `ConnectionTargetResolver`.
2. Set `ResolutionStatus = "failed"` and `ResolutionError = "...agent agent_name is stale and has no recent IP..."` when resolution fails.
3. Return an explicit error or `null` URI from the resolution method, letting callers handle the failure case.
4. Update `AdGuardSyncService`, `MonitoringService`, and other consumers to correctly propagate resolution failures.

## Acceptance Criteria

- [ ] No fallback to `127.0.0.1` on resolution failure
- [ ] Target entry marked with `Status = "failed"` and descriptive `LastError`
- [ ] Connection health reflects unresolved-taget as degraded or failed
- [ ] UI shows resolution error for affected connections
- [ ] Monitoring skips checks for unresolvable targets rather than probing localhost
