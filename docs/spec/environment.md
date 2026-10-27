# Environment (generic)

Facts about the real infrastructure that shape the implementation. This file is public, so it uses roles, not real
names or addresses. The concrete inventory (hostnames, IPs, LXC IDs, paths, which credentials live where) is in
`CLAUDE.local.md` (gitignored) on the developer machine.

> Status: **to be filled from the read-only server survey.** Until then every item below is an assumption.

## Topology

| Role | Count | Notes |
| --- | --- | --- |
| Proxmox host — home | 1 | Linux firewall host candidate; LXCs on a private bridge |
| Proxmox host — VPS | 1 | Linux firewall host candidate; public IP |
| Docker LXC | ≥1 | Runs V1 Hashi today; target for the Hashi container |
| Reverse proxy | ? | Pangolin (+ its Traefik) today — production, special setup |
| Internal DNS | ? | Self-hosted DNS servers (AdGuard Home?) |
| Overlay | NetBird on all firewall hosts | Rescue path |

## External services

| Service | Use | Contract captured |
| --- | --- | --- |
| Hetzner Cloud API (DNS zones/RRSets) | public DNS | no |
| Google Trust Services ACME | certificates | no |
| AdGuard Home | internal rewrites | no |
| Cap | CAPTCHA | no |
| OIDC IdP | edge SSO | no |
| MaxMind | GeoIP | no |
| Gitea (`git.juzo.io`) | repo, CI runners, registry | — |

## Constraints discovered

- Hetzner legacy DNS API is shut down (2026-05-27); a Cloud project token is required.
- (survey findings go here)
