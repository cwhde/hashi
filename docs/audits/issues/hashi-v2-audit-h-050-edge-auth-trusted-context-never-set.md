# H-050: EdgeAuthService TrustedForwardedContext Never Set — All Forward-Auth Denied

**Priority:** Critical
**Conflict Type:** bad_implementation
**Spec Reference:** Main Spec §11, §13

**Status:** Not Started
**Branch:** 

## Description

`EdgeAuthService.EvaluateForwardAsync` in `src/Hashi.Infrastructure/Platform/EdgeAuthService.cs` creates a `SecurityDecisionRequest` without setting `TrustedForwardedContext`:

```csharp
// EdgeAuthService.cs — EvaluateForwardAsync
var request = new SecurityDecisionRequest
{
    // ... other properties set
    // TrustedForwardedContext = NOT SET — defaults to false
};
```

In `SecurityDecisionService` (line ~27), this property is checked first:

```csharp
if (!request.TrustedForwardedContext)
{
    return SecurityDecision.Denied("Request did not come through a trusted proxy context.");
}
```

This means **every single forward-auth evaluation** from Traefik will be denied because `TrustedForwardedContext` is never set to `true` before `SecurityDecisionService` evaluates the request. The forward-auth middleware will always return 403, effectively breaking all edge SSO and adaptive auth flows.

## Evidence

```csharp
// EdgeAuthService.cs — SecurityDecisionRequest created without TrustedForwardedContext = true
var result = await securityDecision.EvaluateForwardAsync(request, cancellationToken);
```

```csharp
// SecurityDecisionService.cs — first check
if (!request.TrustedForwardedContext)
    return SecurityDecision.Denied("not through trusted proxy context");
```

## Expected Outcome

The HTTP pipeline that handles forward-auth requests must set `TrustedForwardedContext = true` on the `SecurityDecisionRequest` before passing it to the `SecurityDecisionService`. This could be done in:
1. The forward-auth HTTP endpoint handler (before calling `EdgeAuthService`)
2. A middleware that validates the request came from the Traefik proxy
3. The `EdgeAuthService` itself, by validating the `X-Forwarded-*` headers or checking the request source against known Traefik connection IPs

If this was intentionally disabled during development, the code should clearly document that edge auth is non-functional and why.

## Fix Guidance

1. In the forward-auth endpoint (`/api/edge-auth/forward`), validate that the request originated from a trusted Traefik instance (check `X-Forwarded-Host` against known connections, or validate source IP).
2. Set `request.TrustedForwardedContext = true` after validation.
3. Or, add a `ForwardedClientContextResolver` check before setting the flag in the endpoint handler.
4. Write integration tests verifying that untrusted forward-auth requests are rejected and trusted ones proceed.

## Acceptance Criteria

- [ ] Forward-auth endpoint sets `TrustedForwardedContext = true` for requests from trusted sources
- [ ] Edge SSO (if implemented) correctly evaluates forward-auth decisions
- [ ] Adaptive auth (challenge, SSO-required, block) works through the forward-auth endpoint
- [ ] Untrusted forward-auth requests are still rejected for security
- [ ] Integration tests cover trusted and untrusted forward-auth scenarios
