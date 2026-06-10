# H-084: Pulse No Reachability Checks on Reported IPs

**Priority:** Medium
**Conflict Type:** missing_implementation
**Spec Reference:** §17.3 (Server records reachability check results)

**Status:** Fixed
**Branch:** audit-series-h

## Description

The spec mentions "reachability check results" for heartbeat data. There is no code that verifies the reported IPs are reachable from the Hashi server. The agent reports IPs and the server accepts them without validation.

## Evidence

- No reachability verification code exists after heartbeat acceptance
- Agent-reported IPs are stored without any connectivity check

## Expected Outcome

After accepting a heartbeat, the server should optionally attempt TCP connection to the reported IPs on the configured port. Reachability status should be recorded on the agent entity. Unreachable agents should show degraded status in the UI.

## Fix Guidance

1. After accepting heartbeat, optionally attempt TCP connection to the reported IPs on the configured port.
2. Record reachability status on the agent entity.
3. Surface reachability state in the UI.

## Acceptance Criteria

- [ ] Reachability check runs after heartbeat acceptance
- [ ] Agent status reflects IP reachability
- [ ] Unreachable agents show degraded status in UI
