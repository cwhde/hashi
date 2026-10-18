# AUD-028: OIDC and Pulse URLs use the backend connection scheme

Priority: P1 — reverse-proxy installation and authentication can fail

Category: Wrong external URL generation / proxy integration

Spec reference: implementation spec §§5.2, 8.4, 16 and 27 require HTTPS admin access, working SSO callbacks, and a functional Pulse install/heartbeat flow.

## Finding

The API is designed to run behind Traefik, which commonly terminates public TLS and forwards HTTP to Hashi. `Program.cs` does not enable ASP.NET forwarded-header processing. OIDC callback URLs and the generated Pulse install instructions build their public base URLs from `HttpRequest.Scheme`; behind TLS termination this remains `http`, even when the external browser request was HTTPS. The Pulse install instructions then persist that HTTP URL for the agent heartbeat endpoint, and OIDC asks the identity provider to use an HTTP callback URL. This can fail provider redirect URI policies and can expose bearer agent tokens over an unencrypted network path or cause POST redirects that break heartbeats.

## Evidence

- `src/Hashi.Api/Program.cs`: no `UseForwardedHeaders` middleware is configured.
- `src/Hashi.Infrastructure/Platform/OidcEdgeAuthService.cs`: `BuildLoginRedirectUri` and `BuildCallbackUrl` use `context.Request.Scheme` directly.
- `src/Hashi.Api/Features/Pulse/PulseEndpoints.cs`: `/api/pulse/agents/{id}/install` uses `ctx.Request.Scheme` to build the API base.
- `src/Hashi.Infrastructure/Platform/PulseInstallRenderer.cs`: the generated Linux and Docker instructions persist that base as `HASHI_PULSE_API`.
- `ForwardedClientContextResolver` resolves client addresses but does not provide a trusted external scheme/host resolver for these URLs.

## Expected outcome

Generate external URLs using the direct request scheme or forwarding metadata validated against the configured trusted-proxy policy. Ensure the same canonical external origin is used for OIDC start/callback and Pulse install/heartbeat URLs. Never accept forwarding headers from untrusted peers.

## Acceptance criteria

- Behind a trusted TLS-terminating proxy, OIDC redirect URIs and Pulse install output use the public HTTPS origin.
- Direct requests cannot spoof the generated scheme or host through arbitrary forwarding headers.
- Pulse heartbeats do not depend on an HTTP-to-HTTPS redirect to preserve POST bodies or tokens.
- OIDC callback URL exactly matches the URL sent in the authorization request.
