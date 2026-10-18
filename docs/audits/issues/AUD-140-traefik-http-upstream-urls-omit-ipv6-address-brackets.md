# AUD-140: Traefik HTTP upstream URLs omit IPv6 address brackets

Priority: P2

Category: Routing / IPv6 upstreams

Status: open; confirmed by manual source trace.

## Finding

HTTP service URLs are interpolated as scheme://host:port without URI construction. A stored IPv6 literal such as2001:db8::1 becomes http://2001:db8::1:8080, an ambiguous/invalid URI. Other parts of the platform explicitly accept IPv6 addresses, so IPv6 resources and route targets cannot reliably work even when YAML syntax is valid.

## Evidence

src/Hashi.Core/Traefik/TraefikConfigRenderer.cs:303-314. AUD-069 covers separate monitoring target parsing; AUD-035 covers unescaped user-controlled configuration syntax.

## Expected outcome and acceptance criteria

Build upstream URIs with an address-family-aware URI builder and validate schemes/ports. IPv4, DNS names and bracketed/unbracketed IPv6 input must yield unambiguous valid URLs.
