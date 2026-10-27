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

## 2. Server model (`pulse_agents`)

Name, ID, token hash, install type, allowed scopes (heartbeat only), heartbeat interval, last seen, last public IP (the
heartbeat's source IP via the client-IP resolver), private IP candidates, selected IP, version, status (fresh / stale /
revoked). Stale threshold setting (default 3× interval).

## 3. Tokens

256-bit random, shown once, stored as `HMAC-SHA256(pepper, token)`; identifies exactly one agent; can only submit that
agent's heartbeat (cannot read config, list resources or trigger syncs); revocable and rotatable. Revoked tokens get `401`.

`POST /api/pulse/{agentId}/heartbeat` — rate limited per agent; validates payload size and IP formats; records reachability
check results (Hashi probes the selected private IP on a configured port, optional).

## 4. Resource and DNS use

Resources and manual DNS entries can target: manual IP, detected firewall host, or a Pulse agent. On an agent IP change:

1. Match internal or public IP against firewall hosts, managed subnets, NetBird-routed subnets, host FQDNs.
2. Match → desired DNS becomes `CNAME → on.<host>`; no match → A/AAAA with the agent's public IP.
3. Queue DNS sync (and Traefik render if the resource's upstream is the agent IP).
4. Status page marks affected endpoints "pending DNS propagation" until the next successful check.

## 5. Agent-bound connection targets

Every connection that needs a host/IP (AdGuard, Traefik host, firewall host where meaningful, future connections) uses a
reusable target model (`connection_targets`):

`target_mode` (static_host / static_ip / pulse_agent), static_host, static_ip, pulse_agent_id, pulse_ip_mode
(`selected` default / `public` / `private_selected` / `private_candidate`), private_candidate_selector (interface name,
CIDR or explicit address), port, scheme, path_prefix, tls_validation_mode, expected_hostname, resolved_ip_snapshot,
last_resolved_at, resolution_status, resolution_error.

UI shows last seen, public IP, private candidates, selected IP, stale status, reachability from Hashi.

Validation before save: agent exists; has a heartbeat (or explicit "save inactive"); IP mode valid; port valid; optional
connectivity test without leaking secrets; unreachable/inactive saves require confirmation.

On resolved-IP change: record the new target, mark dependent connection health pending, queue relevant syncs, touch no
unrelated resources, log dependency impact in Activity.

API: `/api/pulse/agents` CRUD, `POST /api/pulse/agents/{id}/rotate-token|revoke`, `GET /api/pulse/agents/{id}/resolved-targets`,
`GET /api/pulse/agents/{id}/install` (one-liner + compose snippet).

## 6. Releases

Pulse binaries (amd64/arm64) with SHA-256 checksums attached to Gitea releases on tags; multi-arch image published to
`git.juzo.io/juzo/hashi-pulse`.

## 7. Tests that must exist

Token hashing and revocation; heartbeat validation; IP matching → CNAME vs A; agent-bound resolution for each IP mode;
stale handling; installer script shellcheck; image size check in CI.
