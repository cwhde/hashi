# D02 - Forward auth uses proxy IP and drops request context

Priority: High

Spec conflicts: section 11 requires Traefik to send original host, path, method, source IP, and forwarded headers to `/api/edge-auth/forward`. Section 13.2 requires abuse buckets by IP, resource, country, region, ASN, status class, HTTP method, and path prefix.

## Problem

The forward-auth endpoint reads forwarded host and path, but it does not read the forwarded client IP or request method. It uses `HttpContext.Connection.RemoteIpAddress`, which is the proxy or direct connection address unless ASP.NET forwarded-header middleware has already rewritten it. No forwarded-header middleware or trusted proxy configuration is present in the API.

As a result, resource IP/CIDR rules, GeoIP lookup, global blocklist checks, abuse events, and blocklist entries can be evaluated against the Traefik/proxy address instead of the real visitor. Forward-auth decisions are also ingested without the request method, so those events cannot populate the method dimension required by the security bucket model.

## Evidence

- `docs/implementation-spec/hashi-v2-implementation-spec.md:541-547` says Traefik sends original host, original path, method, source IP, and forwarded headers.
- `docs/implementation-spec/hashi-v2-implementation-spec.md:607-619` requires aggregation buckets by IP and HTTP method among other dimensions.
- `src/Hashi.Api/Features/Platform/PlatformEndpoints.cs:387-392` reads `X-Forwarded-Host` and `X-Forwarded-Uri`, but sets `clientIp` from `ctx.Connection.RemoteIpAddress`.
- `src/Hashi.Api/Features/Platform/PlatformEndpoints.cs:411-417` ingests the forward-auth decision without method or path prefix data.
- `src/Hashi.Contracts/Api/PlatformContracts.cs:569-575` defines `ForwardAuthDecisionIngestRequest` without method or region/path-prefix fields.
- `src/Hashi.Infrastructure/Platform/SecurityIngestionService.cs:146-183` stores forward-auth access/security events without method, so the default method path is used rather than the actual forwarded method.
- `src/Hashi.Infrastructure/Platform/EdgeAuthService.cs:23-27` checks the blocklist against the supplied `clientIp`, and `src/Hashi.Infrastructure/Platform/EdgeAuthService.cs:196-210` evaluates resource IP/CIDR/country/region/ASN rules from the same value/context.
- `rg -n "ForwardedHeaders|X-Forwarded-For|X-Real-IP|KnownProxies|UseForwarded" src tests` finds no ASP.NET forwarded-header configuration; only `RemoteIpAddress` and access-log header rendering references appear.

## Expected outcome

Forward auth should evaluate the real client context sent by Traefik, using a trusted forwarded-header strategy. Security events and buckets should record the actual client IP and HTTP method.

## Fix guidance

Configure `ForwardedHeadersOptions` with explicit trusted proxies/networks for the Traefik deployment, or parse `X-Forwarded-For`/`X-Real-IP` in the forward-auth endpoint only after validating the request came from a trusted proxy. Also pass the forwarded or current HTTP method into the forward-auth decision ingestion model and bucket update path.

## Acceptance criteria

- Forward auth uses the real visitor IP from trusted forwarded headers, not the proxy socket IP.
- Untrusted direct requests cannot spoof `X-Forwarded-For`.
- Resource IP/CIDR rules, GeoIP lookup, blocklist checks, and security events use the same trusted client IP.
- Forward-auth decision ingestion records the actual HTTP method and compatible path-prefix data for bucket aggregation.
- Tests cover trusted proxy headers, spoofed untrusted headers, blocklist/resource-rule evaluation, and method aggregation for forward-auth decisions.
