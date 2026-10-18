# AUD-112: Middleware editor accepts arbitrary sibling routing configuration

Priority: P2

Category: Configuration ownership / Traefik

Status: open; confirmed by manual source trace.

## Finding

TraefikUserMiddlewareParser checks only that the first YAML document has http.middlewares as a mapping, then returns the entire original YAML. Sibling http.routers/services, tcp/udp sections, extra documents and conflicting generated object names are not rejected. The stored document is applied verbatim as the user middleware file. Thus a supposedly middleware-only edit can introduce or override routing/auth configuration outside the managed resource model. The actor is an already privileged administrator, not an unauthenticated attacker.

## Evidence

src/Hashi.Core/Traefik/TraefikUserMiddlewareParser.cs:19-63; src/Hashi.Infrastructure/Platform/TraefikUserMiddlewareService.cs:22-49; specification10.3 reserves generated routing files and exposes30-user-middlewares.yml for middleware editing.

## Expected outcome and acceptance criteria

Validate the complete allowed document structure and names, or extract and serialize only the permitted middleware mapping. Reject sibling routers/services, multiple documents and reserved generated names before saving; retain the previous applied file with a clear error.
