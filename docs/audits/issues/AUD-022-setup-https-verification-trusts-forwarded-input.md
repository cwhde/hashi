# AUD-022: Setup HTTPS verification trusts unvalidated forwarded host and scheme

Priority: P2 — setup security invariant can be falsely satisfied

Category: Security / proxy metadata validation

Spec reference: implementation spec §§5.2 and 27 require verifying HTTPS access on the configured admin domain before setup can complete.

## Finding

The verification endpoint reads `X-Forwarded-Proto` directly to decide whether the request was HTTPS, without checking that the peer is a configured trusted proxy. It also accepts any hostname that ends in `.{AdminDomain}`, rather than only the configured admin domain. If a proxy passes client-supplied forwarding headers through, or the setup request is made through an arbitrary subdomain, the endpoint can mark HTTPS verification complete without verifying HTTPS on the exact configured admin hostname.

## Evidence

- `src/Hashi.Api/Features/Setup/SetupEndpoints.cs`: `IsHttpsRequest` trusts the first `X-Forwarded-Proto` value directly.
- `HostMatchesAdminDomain` accepts both an exact match and any subdomain suffix.
- `ForwardedClientContextResolver` demonstrates the project's configured trusted-proxy CIDR policy, but the HTTPS verification endpoint does not use it.
- `SetupCompletionService.TryCompleteAsync` only checks whether `HttpsDomainVerifiedAtUtc` is non-null.

## Expected outcome

Verify the exact configured admin host and derive the external scheme only from the direct HTTPS connection or a forwarding header received from a configured trusted proxy that overwrites client-supplied values.

## Acceptance criteria

- Direct HTTP requests cannot satisfy HTTPS verification by supplying `X-Forwarded-Proto: https`.
- A trusted reverse proxy can report the original HTTPS scheme through a documented, validated mechanism.
- A sibling or nested subdomain cannot satisfy verification for the configured admin domain.
- Verification state is set only after both exact-host and scheme checks pass.
