# AUD-027: Pulse heartbeat records the reverse proxy as the agent public IP

Priority: P2 — agent target and DNS correctness behind reverse proxies

Category: Wrong request metadata source

Spec reference: implementation addendum §§6–8 requires Pulse target resolution and internal DNS to use agent IP candidates and public address metadata.

## Finding

The public heartbeat endpoint passes `HttpContext.Connection.RemoteIpAddress` directly to `PulseAgentService` as `remotePublicIp`. When Hashi is deployed behind a reverse proxy, that socket address is the proxy, not the agent. `PulseAgentService` stores it as `LastPublicIp`, which is then used for DNS generation, target selection, and AdGuard rewrites. Multiple agents can therefore inherit the same proxy address and cause incorrect routing/DNS.

## Evidence

- `src/Hashi.Api/Features/Pulse/PulseEndpoints.cs`: heartbeat handler passes `ctx.Connection.RemoteIpAddress?.ToString()` directly.
- `src/Hashi.Infrastructure/Platform/PulseAgentService.cs`: `AcceptHeartbeatAsync` normalizes the supplied value into `publicIp` and stores it as `LastPublicIp` when available.
- `ForwardedClientContextResolver` already contains the trusted proxy CIDR policy, but the heartbeat endpoint does not use it.

## Expected outcome

Resolve the connecting agent address from the socket peer and forwarded chain using the trusted-proxy policy. Ignore forwarded client metadata from untrusted peers, and define how a proxy should report the agent's public address when direct client IP is not a suitable target.

## Acceptance criteria

- Direct agent requests record the direct peer address.
- Requests from configured proxies can resolve the originating agent address from a validated forwarded chain.
- Untrusted peers cannot spoof the stored public address with forwarding headers.
- A reverse-proxy integration does not assign the same proxy IP to unrelated agents.
