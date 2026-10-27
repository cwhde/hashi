# Environment

Facts about the real infrastructure that shape the implementation, from the read-only survey on 2026-10-27.
Concrete hostnames, addresses and paths are in `CLAUDE.local.md` (gitignored).

## Topology today

| Role | Instance | Notes |
| --- | --- | --- |
| Edge host A (VPS) | Proxmox, public IP, NetBird | Managed firewall host. All public traffic enters here. Runs a cron-driven firewall script today. |
| Edge host B (home) | Proxmox behind a home NAT router | No host firewall, no NetBird on the host. Public traffic for its services is routed via host A (`on.B → via.A`). Host A reaches B's LAN through a NetBird network route. |
| Reverse proxy A | Traefik v3.7 in Docker on a dedicated LXC (host network) | Today configured by Pangolin's HTTP provider + Pangolin's badger auth plugin. |
| Reverse proxy B | Traefik v3.7 native on an Alpine LXC (OpenRC) | Same static config as A, also pulls routes from Pangolin. |
| Internal DNS | Two AdGuard Home instances | One is the adguardhome-sync **origin**, the other a replica (sync every 10 min). |
| Docker host | LXC on host A, compose projects managed by Arcane | Runs V1 Hashi, Gatus, Pocket ID, NetBird server, Gitea + runners. Hashi deploys here. |
| IdP | Pocket ID (OIDC) | Candidate default edge-SSO provider. |
| CAPTCHA | wicketkeeper (not Cap) on its own domain/port | See open question in README. |

## What Hashi replaces

Everything below is retired once Hashi runs: V1 (DNS CNAMEs from a topology TXT record + Pangolin resources; Gatus
endpoint file), Gatus, Pangolin and both current Traefik instances (Hashi installs its own), adguardhome-sync, and the
host-A firewall script. Their current configuration is reference material only, not something Hashi adopts.

## External services

| Service | Use | Contract captured |
| --- | --- | --- |
| Hetzner Cloud API (zones, RRSets) | public DNS, 2 zones | shape observed (zones, rrsets, pagination); fixtures pending |
| Google Trust Services ACME (EAB) | certificates | — |
| AdGuard Home ×2 (+ adguardhome-sync) | internal rewrites | — |
| Pocket ID | edge SSO | — |
| MaxMind GeoLite2 | GeoIP (Pangolin already downloads Country) | — |
| Gitea (`git.juzo.io`) | repo, act_runner CI, container registry | — |

## Constraints discovered

- Hetzner legacy DNS API is shut down; the Cloud API works with the existing project token.
- Hetzner RRSets carry `labels` (key/value) and a `protection.change` flag; records carry a `comment`; `ttl: null` means zone default.
- Host B has no public IP of its own on the host: the firewall-host model needs an "external NAT" mode (05-firewall.md §1.1).
- The current firewall script re-inserts its NetBird forward rules on every run (≈30 duplicates observed) — Hashi's check-before-insert design must not repeat this.
- The current firewall script also maintains Proxmox corosync peer MTU routes and an `/etc/hosts` pin for the NetBird management host; these behaviours move into Hashi (05-firewall.md §3.2).
- A topology TXT record is consumed by the current firewall script; it must keep existing until Hashi's firewall slice replaces the script.
- Ports 8080–8082 are taken on the Docker host; Hashi's listener ports are configurable and the deployment uses free ones.
- Both Traefik instances use `serversTransport.insecureSkipVerify: true` and log at DEBUG; Hashi's static config will tighten both per resource.
