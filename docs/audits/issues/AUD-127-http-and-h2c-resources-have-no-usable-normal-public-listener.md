# AUD-127: HTTP and H2C resources have no usable normal public listener

Priority: P1

Category: Routing / protocol implementation

Status: open; confirmed by manual source trace.

## Finding

Static configuration redirects port80 to HTTPS443. Every HTTP-family router is attached only to websecure, but TLS is enabled only for ResourceKind.Https. Http/H2c resources therefore have a plaintext-only router on port443 while normal port80 access redirects the browser to TLS, which cannot select that router. The upstream protocol and public listener policy are conflated. Resource kind Http/H2c is accepted and displayed as a supported feature.

## Evidence

src/Hashi.Core/Traefik/TraefikConfigRenderer.cs:91-101,335-369. Check intended public scheme against main specification resource protocol support. This is separate from the already noted custom ACME resolver mismatch AUD-016.

## Expected outcome and acceptance criteria

Define public listener/TLS independently from upstream scheme and generate a reachable route for every supported HTTP-family kind. Validate normal public URLs for HTTP, HTTPS and H2C backends through real pinned Traefik, including redirects and certificate selection.
