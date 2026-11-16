# Hashi (橋)

A single-user control plane for a distributed homelab edge: public DNS, Traefik reverse proxy, Linux firewall
forwarding with NetBird, internal DNS rewrites, monitoring with public status and app pages, edge SSO, abuse response,
host scripts and dynamic endpoint discovery — one interface instead of hand-editing zones, proxy files and firewall scripts.

**Status:** in production on its author's homelab; the last slice before 1.0 (S12, hardening) is in progress. See
[docs/progress.md](docs/progress.md), the specification in [docs/spec/](docs/spec/README.md), how to run it in
[docs/operations.md](docs/operations.md) and the [threat model](docs/threat-model.md).

## Progress

<!-- progress:start — update when a slice passes its prod-proof gate (CLAUDE.md → "Progress bar") -->
```text
[██████████████████████████████]  15 / 15 steps  ·  100 %

done   S0 walking skeleton · S0b UI design · S1 setup & auth · S2 DNS read · S3 DNS write · S4 SSH & Traefik · S5 HTTP services · S6 firewall · S7 monitoring · S8 AdGuard & Pulse · S9 edge SSO · V1 cutover · S10 security engine · S11 scripts & themes · S12 hardening
next   bug hunt round
then   v1.0.0
```
<!-- progress:end -->

Stack: Go · SQLite · SvelteKit (embedded SPA) · one container. Pulse agent: Go.

License: MIT.
