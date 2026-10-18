# AUD-008: Firewall SSH-risk check trusts an unvalidated forwarded header

Priority: P2 — firewall safety guard

Spec reference: implementation spec §14.4–14.5 requires warning before a firewall change can remove the operator's remote access.

## Finding

Firewall planning and apply logic take the first `X-Forwarded-For` value directly from the request and use it as the operator's IP. They do not use `ForwardedClientContextResolver`, which only honors forwarding headers when the immediate peer is a configured trusted proxy. A direct caller can therefore spoof an address that is allowed by the planned firewall. The apply guard then skips the “would block your current connection” warning and does not require `AcknowledgeSshBlockRisk`, even if the real caller address would be blocked. The global sync plan and apply paths repeat this raw-header parsing, so both preview and enforcement can use a spoofed address.

## Evidence

- `src/Hashi.Infrastructure/Platform/FirewallApplyService.cs`, in `PlanForHostAsync` and `ApplyDefinitionAsync`, reads `X-Forwarded-For` before falling back to `RemoteIpAddress`.
- `src/Hashi.Infrastructure/Sync/SyncOrchestratorService.cs`, in `PlanGlobalAsync` and `ApplyGlobalAsync`, repeats the same raw-header parsing for global firewall risk checks.
- `src/Hashi.Api/Hosting/ForwardedClientContextResolver.cs` contains the trusted-proxy-aware resolver used by authentication and edge-auth paths, but the firewall service does not use it.
- `ApplyDefinitionAsync` only requires acknowledgment when `IsIpAllowed(clientIp, definition)` returns false.

## Expected outcome

Firewall plan and apply must use a canonical client IP derived from the connection and trusted-proxy configuration. Untrusted callers must not be able to suppress the lockout warning by supplying forwarding headers.

## Acceptance criteria

- Direct requests with spoofed forwarding headers are evaluated using the socket peer address.
- Requests through configured proxies use the canonical client address resolved by the shared resolver.
- Plan and apply use the same resolved address and produce the same lockout warning decision.
