# Hashi (橋)

A single-user control plane for a distributed homelab edge: public DNS, Traefik reverse proxy, Linux firewall
forwarding with NetBird, internal DNS rewrites, monitoring with public status and app pages, edge SSO, abuse response,
host scripts and dynamic endpoint discovery — one interface instead of hand-editing zones, proxy files and firewall scripts.

**Status:** restarting implementation. See [docs/progress.md](docs/progress.md) and the specification in
[docs/spec/](docs/spec/README.md). The previous Node version still runs production from [legacy/v1](legacy/v1/README.md).

Stack: Go · SQLite · SvelteKit (embedded SPA) · one container. Pulse agent: Go.

License: MIT.
