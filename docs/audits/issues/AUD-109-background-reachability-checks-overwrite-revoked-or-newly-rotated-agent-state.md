# AUD-109: Background reachability checks overwrite revoked or newly rotated agent state

Priority: P2

Category: Reliability / Pulse lifecycle

Status: open; confirmed by manual source trace.

## Finding

Every accepted heartbeat starts an untracked task that probes TCP22 and later overwrites Status unconditionally. Revocation or rotation may occur while the probe waits; its later save changes revoked/pending to online/degraded. TokenHash is not restored, so this does not resurrect the old token, but UI/monitoring lifecycle and rotation guards now see a misleading active agent. Concurrent heartbeats also have no task coalescing and a healthy agent without SSH22 is classified degraded.

## Evidence

src/Hashi.Infrastructure/Platform/PulseAgentService.cs:57-70,161-173,183-196,441-470. The task re-queries the row but never checks token generation or status before overwriting it.

## Expected outcome and acceptance criteria

Separate heartbeat health, optional service reachability and authorization lifecycle. Cancel/coalesce obsolete probes and use generation-aware conditional updates. Revoke/rotate during a delayed probe must preserve revoked/pending state and old tokens must remain invalid.
