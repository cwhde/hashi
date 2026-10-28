# ACME (RFC 8555)

Captured 2026-10-28, read-only GETs of the public directories:

- `directory_gts.json` — Google Trust Services (`externalAccountRequired: true`; the same URL is the `caServer` of the
  Traefik config Hashi replaces).
- `directory_letsencrypt.json` — Let's Encrypt (no EAB).

The protocol itself (accounts with External Account Binding, orders, DNS-01, finalize, certificate download) is
exercised against Pebble 2.10.1 (`ghcr.io/letsencrypt/pebble`, EAB required, with `pebble-challtestsrv` as its DNS) in
`internal/certs` integration tests, and against Google Trust Services at the S4 gate.
