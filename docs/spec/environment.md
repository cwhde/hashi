# Environment

Facts about the real infrastructure that shape the implementation, from the read-only survey on 2026-10-27.
Concrete hostnames, addresses and paths are in `CLAUDE.local.md` (gitignored).

## Topology today

| Role | Instance | Notes |
| --- | --- | --- |
| Edge host A (VPS) | Proxmox, public IP, NetBird | Managed firewall host. All public traffic enters here. Runs a cron-driven firewall script today. |
| Edge host B (home) | Proxmox behind a home NAT router | No host firewall. NetBird runs on the home **router** (routing peer for the home LAN), not on the host. Public traffic for its services is routed via host A (`on.B → via.A`); host A reaches B's LAN through that NetBird network route. |
| Reverse proxy A | Traefik v3.7 in Docker on a dedicated LXC (host network) | Today configured by Pangolin's HTTP provider + Pangolin's badger auth plugin. |
| Reverse proxy B | Traefik v3.7 native on an Alpine LXC (OpenRC) | Same static config as A, also pulls routes from Pangolin. |
| Internal DNS | Two AdGuard Home instances | One is the adguardhome-sync **origin**, the other a replica (sync every 10 min). |
| Docker host | LXC on host A, compose projects managed by Arcane | Runs V1 Hashi, Gatus, Pocket ID, NetBird server, Gitea + runners. Hashi deploys here. |
| IdP | Pocket ID (OIDC) | Candidate default edge-SSO provider. |

## What Hashi replaces

Everything below is retired once Hashi runs: V1 (DNS CNAMEs from a topology TXT record + Pangolin resources; Gatus
endpoint file), Gatus, Pangolin and both current Traefik instances (Hashi installs its own), adguardhome-sync, and the
host-A firewall script. Their current configuration is reference material only, not something Hashi adopts.

## External services

| Service | Use | Contract captured |
| --- | --- | --- |
| Hetzner Cloud API (zones, RRSets) | public DNS, one zone managed | read side 2026-10-28: `testdata/contracts/hetzner/` (`make contract-capture PROVIDER=hetzner`); writes 2026-10-28 with `_hashi-test` probe RRSets: `testdata/contracts/hetzner/write/` (`tools/contract-capture/hetzner_write.py`) |
| Traefik v3.7.16 (release binary, file provider, API) | reverse proxy on the edge LXCs | 2026-10-28 on a disposable Alpine LXC: `testdata/contracts/traefik/` (release checksums, `/api/rawdata`, cert reload, forward-auth fallback) |
| Edge host OS probe over SSH | Traefik/firewall host validation | 2026-10-28, Alpine 3.23 and Debian 12: `testdata/contracts/os/` |
| Google Trust Services ACME (EAB) | certificates | — |
| AdGuard Home ×2 (+ adguardhome-sync) | internal rewrites | — |
| Pocket ID | edge SSO | — |
| MaxMind GeoLite2 | GeoIP (Pangolin already downloads Country) | — |
| Gitea (`git.juzo.io`) | repo, act_runner CI, container registry | — |

## Constraints discovered

- Hetzner legacy DNS API is shut down; the Cloud API works with the existing project token.
- Hetzner RRSets carry `labels` (key/value) and a `protection.change` flag; records carry a `comment`; `ttl: null` means zone default.
- Hetzner Cloud DNS (captured 2026-10-28): RRSet `id` is `<name>/<TYPE>` with relative names (`@` for the apex); a zone can
  be read by id **or** by name (`GET /v1/zones/{name}` → 200); pagination is `meta.pagination` with `next_page: null` on the
  last page; rate limit 3600 requests per hour via `RateLimit-Limit/-Remaining/-Reset` (reset is a Unix time); errors are
  `{"error": {"code", "message"[, "details"]}}` (`unauthorized` 401 without `details`, `not_found` 404 with `details: null`);
  a zone's `record_count` counts records, not RRSets (64 vs 58); TXT values are stored quoted, several quoted strings per value.
- Hetzner Cloud DNS writes (probe capture 2026-10-28): create (`POST …/rrsets`), `set_records`, `add_records`,
  `remove_records`, `change_ttl` (`ttl: null` restores the zone default) and `DELETE` all answer 201 with an **Action**
  (`running` at first; poll `GET /v1/actions/{id}` until `success`/`error`); label changes are a synchronous `PUT` (200,
  `hashi.io/owner` accepted as a key). `set_records` with unchanged records still creates an action, so Hashi must diff
  first. Values come back exactly as sent. TXT values must be quoted (`422 invalid_input` otherwise) and no single string
  may exceed 255 characters (422) — Hashi splits. Existing name+type → `409 uniqueness_error` with `details.fields`;
  CNAME beside other types → 422; invalid A/SRV/CAA values → `422 invalid_input` ("invalid value '…'"); an unknown type →
  422 on the field. Missing RRSet → 404 `not_found`.
- Host B has no public IP of its own on the host: the firewall-host model needs an "external NAT" mode (05-firewall.md §1.1).
- The current firewall script re-inserts its NetBird forward rules on every run (≈30 duplicates observed) — Hashi's check-before-insert design must not repeat this.
- The current firewall script's chains, ipset and cron entries are removed by Hashi's first firewall apply (05-firewall.md §3.4); its MTU-route and `/etc/hosts` workarounds are obsolete and not carried over.
- A topology TXT record is consumed by the current firewall script; it must keep existing until Hashi's firewall slice replaces the script.
- Ports 8080–8082 are taken on the Docker host; Hashi's listener ports are configurable and the deployment uses free ones.
- Both Traefik instances use `serversTransport.insecureSkipVerify: true` and log at DEBUG; Hashi's static config will tighten both per service.
- Traefik v3.7.16 (captured 2026-10-28, `testdata/contracts/traefik/README.md`): changed certificate files are reloaded only
  when the dynamic file that lists them changes; `noop@internal` answers 418 (denies as a forward-auth fallback),
  `ping@internal` answers 200 on any path; a loopback entry point used as a forward-auth hop must trust loopback
  forwarded headers or the original host/URI are lost; GitHub's release list mixes v3 and v2 maintenance releases, so
  "latest" means the highest version of the installed major.
