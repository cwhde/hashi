# AUD-098: Forward-auth rate-limit state is never produced

Priority: P1

Category: Missing security feature / rate limiting

Status: open; confirmed by manual producer-consumer tracing.

## Finding

The decision engine can return 429 only when `SecuritySubjectStateEntity.RateLimitedUntilUtc` is in the future. Nothing in production assigns that field or increments `RateLimitRequestCount`: repository-wide references are declarations, model mappings and reads/projections. No windowed request-bucket calculation produces this state. Thus the subject-aware limiter advertised by its 429 branch and manual bypass controls cannot activate through normal requests.

Traefik does render baseline and profile rate-limit middlewares. This finding concerns the missing Hashi decision-engine path; it does not claim the reverse proxy has no rate limiting. The independent proxy limiter also cannot implement the promised subject-aware manual `BypassRateLimit` semantics by merely adding an unreachable engine branch.

## Evidence

- `src/Hashi.Infrastructure/Platform/SecurityDecisionService.cs:190-199,723-759`: reads state and returns 429.
- `src/Hashi.Infrastructure/Persistence/Entities/ExtendedPlatformEntities.cs:408-410`: persisted state fields.
- `src/Hashi.Api/Features/EdgeAuth/EdgeAuthEndpoints.cs:101`: response mapping only.
- `src/Hashi.Core/Traefik/TraefikConfigRenderer.cs:234,274-289,588`: separate proxy rate-limit middleware.
- Main specification §11/§13 and addendum security decision requirements; historical H-062/H-096 are only partially resolved. AUD-087 separately records incorrectly applied bypass flags.

## Expected outcome

Implement bounded, concurrent request-rate accounting and expiry in the decision path, tied to configured limits and scopes. Apply selective bypass semantics and preserve ordering relative to challenge, hard blocks and SSO.

## Acceptance criteria

Normal traffic can actually reach the 429 branch after exceeding a configured window, recover after expiry, and selectively bypass rate limiting only when authorized. Exercise the real request path without directly seeding a future rate-limit state.
