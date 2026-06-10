# H-062: Forward Auth Missing Rate-Limit Response — No 429 Status Code for Rate-Limited Traffic

**Priority:** High
**Conflict Type:** missing_implementation
**Spec Reference:** Main Spec §11 (Forward auth returns 429 for rate-limited/challenged traffic); Addendum §14 (Decision actions include rate-limiting)

**Status:** Not Started
**Branch:** 

## Description

`SecurityDecisionService.DecideForwardAuthAsync()` never returns a 429 status code. All deny responses return 403. Rate limiting exists as Traefik middleware (`hashi-rate-limit`) but is not part of the forward-auth decision flow. The `BypassRateLimit` flag exists on manual entries but no rate-limit enforcement or 429 response exists in the decision engine. The spec requires forward-auth to return 429 for rate-limited traffic.

## Evidence

SecurityDecisionResponseModeNames has no rate-limit response mode. SecurityDecisionService only produces allow, deny, challenge, redirect decisions. No 429 response code exists in the forward-auth endpoint handler.

## Expected Outcome

When a subject exceeds request rate limits, the decision service should return HTTP 429 Too Many Requests, distinct from 403 Forbidden (blocked) and challenge responses.

## Fix Guidance

Add rate-limit checking step in DecideForwardAuthAsync that evaluates request buckets and returns a 429 response mode when thresholds are exceeded. Add a `rate_limited` response mode to SecurityDecisionResponseModeNames. Wire this into the edge-auth/forward endpoint.

## Acceptance Criteria

- [ ] Excessive requests from a single subject produce HTTP 429
- [ ] 429 responses are distinct from 403 block responses
- [ ] Manual allow with bypassRateLimit=true suppresses the 429 response
