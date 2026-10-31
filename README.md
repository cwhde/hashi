# Hashi (橋)

A single-user control plane for a distributed homelab edge: public DNS, Traefik reverse proxy, Linux firewall
forwarding with NetBird, internal DNS rewrites, monitoring with public status and app pages, edge SSO, abuse response,
host scripts and dynamic endpoint discovery — one interface instead of hand-editing zones, proxy files and firewall scripts.

**Status:** walking skeleton deployed next to V1 and the UI designed for every feature; setup and auth (S1) is next. See [docs/progress.md](docs/progress.md) and the specification in
[docs/spec/](docs/spec/README.md). The previous Node version still runs production from [legacy/v1](legacy/v1/README.md).

## Progress

<!-- progress:start — update when a slice passes its prod-proof gate (CLAUDE.md → "Progress bar") -->
```text
[████████████████░░░░░░░░░░░░░░]  8 / 15 steps  ·  53 %

done   S0 walking skeleton · S0b UI design · S1 setup & auth · S2 DNS read · S3 DNS write · S4 SSH & Traefik · S5 HTTP services · S6 firewall
next   S7 monitoring · V1 cutover
then   S8 AdGuard & Pulse · S9 edge SSO · S10 security engine · S11 scripts & themes · S12 1.0
```
<!-- progress:end -->

Stack: Go · SQLite · SvelteKit (embedded SPA) · one container. Pulse agent: Go.

License: MIT.
