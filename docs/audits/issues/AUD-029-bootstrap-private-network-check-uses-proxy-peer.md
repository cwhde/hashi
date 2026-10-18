# AUD-029: Bootstrap private-network gate checks the proxy peer

Priority: P2 — bootstrap network restriction can be bypassed behind a private reverse proxy

Category: Security / proxy-aware client identity

Spec reference: implementation spec §7.1 requires bootstrap login to be allowed only from private/internal client ranges by default.

## Finding

The bootstrap login endpoint passes `HttpContext.Connection.RemoteIpAddress` directly to `BootstrapNetworkPolicy.IsAllowed`. When Hashi is reached through a reverse proxy on a private or Docker bridge address, the socket peer is the proxy and falls in an allowed range even when the actual visitor is public. The endpoint already has a trusted-proxy-aware `ForwardedClientContextResolver`, but this gate does not use it. The bundled Compose file publishes the admin port directly; deployments that instead route the admin endpoint through a private proxy can therefore lose the spec's source-network restriction.

## Evidence

- `src/Hashi.Api/Features/Auth/AuthEndpoints.cs`: `/api/auth/bootstrap/login` applies the network check to `Connection.RemoteIpAddress` before resolving the client context later for session creation.
- `src/Hashi.Infrastructure/Bootstrap/BootstrapInitializer.cs`: `BootstrapNetworkPolicy` permits RFC1918 IPv4 ranges including `172.16.0.0/12`, which includes common container bridge networks.
- `src/Hashi.Api/Hosting/ForwardedClientContextResolver.cs`: the application has a trusted-proxy-aware resolver that can determine the caller address, but the bootstrap network gate bypasses it.
- `docs/implementation-spec/hashi-v2-implementation-spec.md` §7.1: the private/internal range restriction applies to bootstrap login.

## Expected outcome

Apply the bootstrap network policy to the canonical client IP resolved through the configured trusted-proxy policy. Ignore forwarded headers from untrusted peers, and fail closed when a trusted proxy does not provide a valid client address.

## Acceptance criteria

- A public client reaching Hashi through an allowed private proxy address is rejected unless the resolved client address itself is in an allowed range.
- A private client reaching Hashi through a configured trusted proxy is allowed.
- Untrusted peers cannot forge forwarded addresses to satisfy the policy.
- Missing or malformed forwarding data from a trusted proxy fails closed.
