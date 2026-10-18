# AUD-149: Forward auth chooses the first matching host and ignores resource path routing

Priority: P1

Category: Security / resource identity

Status: open; confirmed by manual source trace.

## Finding

BuildContextAsync chooses FirstOrDefault among enabled resources with the resolved host, without considering request path, route match type or Traefik priority. Multiple resources can share a domain with different paths and policies. The router can therefore send a request to one resource while Hashi evaluates another resource's rules/manual scope/provider/CAP identity. Strict mode enforces SSO generically but does not repair wrong resource-specific rules or scopes. Database enumeration order is not a deterministic routing policy.

## Evidence

src/Hashi.Infrastructure/Platform/SecurityDecisionService.cs:296-313; Core/Traefik/TraefikConfigRenderer.cs:335-352; ResourceService accepts path/routing configuration.

## Expected outcome and acceptance criteria

Carry a trustworthy resource identity from generated middleware or resolve the exact host/path/priority semantics consistently. Two same-host resources with distinct access rules must never inherit each other's allow, bypass, challenge or provider state.
