# 08 — Hashi Pulse and agent-bound connections

Pulse lets a host report its current addresses without holding any provider credentials.

## 1. Agent

- Go, static binary (`CGO_ENABLED=0`), linux/amd64 + linux/arm64; Docker image on distroless/scratch, < 10 MB compressed.
- Linux install: one-line command (shown in UI) that downloads the binary from the Hashi release (checksum verified),
  writes root-owned `/etc/hashi-pulse/config.yaml` (0600), installs a systemd service + timer, or cron fallback.
- Docker install: generated Compose snippet (`network_mode: host` recommended for interface discovery).
- Config: Hashi URL, agent ID, token, interval, optional interface selector.
- Heartbeat: agent version, hostname, private IPv4/IPv6 candidates (per interface), optional user-selected interface,
  timestamp, optional Docker metadata. Retries with jittered backoff.
- Footprint: small enough to run in every Compose stack, dozens of instances per host. Targets, measured in S8: idle
  RSS ≤ 15 MB, no disk writes after start (config read once, no local state), sleeps between heartbeats (no busy
  loops or polling). Pulse stays heartbeat-only; firewall or other root-level host functions never go into it
  (ADR 0002, "Host link vs host agent").

## 2. Server model (`pulse_agents`)

Name, ID, token hash, install type, allowed scopes (heartbeat only), heartbeat interval, last seen, last public IP (the
heartbeat's source IP via the client-IP resolver), private IP candidates, selected IP, version, status (fresh / stale /
revoked). Stale threshold setting (default 3× interval).

Reachability check (optional, per agent): Hashi itself tries to open a TCP connection to the agent's selected IP on a
configured port, so the UI can show "Hashi can reach this address" vs "agent reports it but Hashi can't reach it".

## 3. Tokens

256-bit random, shown once, stored as `HMAC-SHA256(pepper, token)`; identifies exactly one agent; can only submit that
agent's heartbeat (cannot read config, list services or trigger syncs); revocable and rotatable. Revoked tokens get `401`.

`POST /api/pulse/{agentId}/heartbeat` — rate limited per agent; validates payload size and IP formats; records reachability
check results (Hashi probes the selected private IP on a configured port, optional).

## 4. Pulse in IP sets

A Pulse agent is used everywhere through a `pulse` IP item (05a-ip-sets.md §2) — scope `public` (heartbeat source
address), `internal` (private candidates, selected one first, optional interface/CIDR selector) or `all`. That one item
type makes an agent usable in every position: firewall allow/block and host access, rule `Applies To`, DNS record values,
service upstreams and connection targets. There is no separate agent-bound target model.

On an address change (heartbeat with new public address or new private candidates):

1. Recompile every IP set that contains the agent (directly or nested) and notify their consumers (05a §4.2).
2. Firewall consumers: the folded `allow`/`block` and host access are re-planned (05-firewall.md §4.4).
3. DNS consumers: services whose target set contains the agent re-evaluate host matching (03-dns.md §5.2: match →
   `CNAME → on.<host>`, no match → A with the public IPv4); manual records with set values follow the new addresses.
4. Traefik consumers: services whose upstream set contains the agent get a Traefik sync, so Traefik forwards to the new
   address — updating DNS alone would leave Traefik pointing at the old one.
5. Connection consumers: connections whose target set contains the agent mark health pending and reconnect.
6. Status page marks affected endpoints "pending DNS propagation" until the next successful check.

Nothing unrelated is touched; Activity logs the dependency impact (which sets, which consumers).

## 5. Connection targets

Every connection that needs a host/IP (Traefik host SSH, firewall host SSH, AdGuard, future connections) takes an
**address set** plus port, scheme, path prefix, TLS validation mode and expected hostname. Addresses are tried in item
order. The UI shows, per resolved address, its source (e.g. Pulse agent: last seen, public address, private candidates,
selected address, stale status, reachability from Hashi).

Validation before save: the set is an address set; Pulse agents in it exist and have a heartbeat (or explicit "save
inactive"); port valid; optional connectivity test without leaking secrets; unreachable/inactive saves require
confirmation. A target that resolves to nothing keeps the last working address (invariant 21).

Former `pulse_ip_mode` values map to item scopes: `selected` / `private_selected` → `internal` (selected first),
`public` → `public`, `private_candidate` → `internal` with a selector.

API: `/api/pulse/agents` CRUD, `POST /api/pulse/agents/{id}/rotate-token|revoke`, `GET /api/pulse/agents/{id}/consumers`
(IP sets containing the agent and their consumers), `GET /api/pulse/agents/{id}/install` (one-liner + compose snippet).

## 6. Releases

Pulse binaries (amd64/arm64) with SHA-256 checksums attached to Gitea releases on tags; multi-arch image published to
`git.juzo.io/juzo/hashi-pulse`.

## 7. Tests that must exist

Token hashing and revocation; heartbeat validation; IP matching → CNAME vs A; `pulse` item resolution for each scope and
selector; a heartbeat change reaches every consumer type (firewall, DNS, Traefik, connection);
stale handling; installer script shellcheck; image size check in CI.
