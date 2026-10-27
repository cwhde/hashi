# 05 — Linux firewall hosts

A Linux firewall host is a machine with a public IP (typically a Proxmox host) that forwards public ports into managed
subnets where Traefik runs. Hashi generates and maintains its firewall script, keeps NetBird working, and protects the
rescue path.

## 1. Host model

Name (→ public FQDN `name.<root>`), public IP (detected or override), public IPv6 (optional), managed subnets (multiple
CIDRs), linked Traefik connection, internal Traefik target IP, route target override (`on.` record, 03-dns.md §5.1),
SSH target + credentials + pinned host key, OS family, WAN interface (detected or override), LXC bridge (detected or
override), NetBird settings (§4), trusted allowlist entries, persistence mode.

### 1.1 Edge modes

| Mode | When | Hashi manages |
| --- | --- | --- |
| `managed` | Host owns its public IP (e.g. a VPS) | Everything in this file |
| `external_nat` | Host sits behind a router/NAT it doesn't control (e.g. home) | No firewall artifacts. Host records still exist (public IP detected from the host's egress or set manually); the UI shows which ports must be forwarded on the router, and the typical setup routes its `on.` record via a `managed` host. |

## 2. Setup and validation

Collect: name, SSH credentials (same flow as Traefik), managed subnets, linked Traefik connection, internal Traefik IP,
optional WAN interface / bridge / public IP overrides.

Validate: SSH login + host-key pin; OS detection; presence of `iptables` (and whether it is the nft or legacy backend),
`ipset`, `ip`, `sysctl`, persistence tooling; public IP detection (from the host itself, cross-checked with Hashi's view);
managed subnets are valid, non-overlapping, and contain the internal Traefik IP; NetBird detection (§4).

**Read the existing ruleset first.** Setup captures `iptables-save`, `ip6tables-save`, `ipset save`, `nft list ruleset`
and shows them; Hashi's first plan is computed relative to this real state.

## 3. Generated artifacts

| Path | Purpose |
| --- | --- |
| `/opt/hashi/firewall/hashi-firewall.sh` | Idempotent apply script (POSIX sh; shellcheck-clean) |
| `/opt/hashi/firewall/hashi-firewall.env` | Rendered parameters (no secrets) |
| `/opt/hashi/firewall/rollback.sh` + `previous.rules` | Rescue restore (mode 0700) |
| systemd timer `hashi-firewall.timer` / `/etc/cron.d/hashi-firewall` | Periodic refresh (trusted FQDN IPs) |
| boot unit `hashi-firewall.service` / OpenRC service | Re-apply at boot after network-online |

Packages are ensured by the apply plan, not assumed:
Debian/Ubuntu `iptables ipset iptables-persistent netfilter-persistent`; Alpine `iptables ipset` + OpenRC persistence
(`/etc/iptables/rules-save` via the `iptables` service).

### 3.1 What the script does

1. Detect WAN interface and public IP unless configured.
2. Enable IPv4 forwarding (`net.ipv4.ip_forward=1`, persisted in `/etc/sysctl.d/90-hashi.conf`).
3. Build ipsets **as temporary sets and `ipset swap` them in** — live sets are never flushed before replacements are ready:
   `hashi_trusted` (all firewall hosts' public IPs resolved from `name.<root>`, current host IP, user allowlist),
   `hashi_blocked` / `hashi_blocked6` (firewall-enforced blocks, 06-edge-security.md), `hashi_netbird` (overlay CIDRs).
4. Build chains via `iptables-restore --noflush` with only Hashi chains in the input, so the swap is atomic per table:
   - `filter`: `HASHI_INPUT`, `HASHI_FWD`, `HASHI_NETBIRD`
   - `nat`: `HASHI_DNAT` (PREROUTING), `HASHI_POSTROUTING` (POSTROUTING) — NAT chains live in the **nat** table
   - Jump rules from built-in chains are inserted once, checked with `-C` before `-I`.
5. INPUT policy inside `HASHI_INPUT`: allow loopback, established/related, managed subnets, `hashi_trusted`, NetBird
   overlay on the NetBird interface, configured public ports; drop `hashi_blocked` first; final drop for non-allowed new
   inbound traffic only after the user accepted the "block all other inbound" warning.
6. DNAT each public TCP/UDP port to the linked internal Traefik IP **and the Traefik entry-point port** for that resource
   (80/443 for HTTP family, the stream entry point port for TCP/UDP — not the upstream service port).
7. `HASHI_FWD`: drop `hashi_blocked` for **forwarded** traffic too, allow DNATed flows, allow
   managed-subnet egress, established/related.
8. `HASHI_POSTROUTING` (nat): masquerade managed subnets → WAN; hairpin NAT so internal clients can reach public endpoints;
   NetBird masquerade rules for routing-peer mode in the nat table.
9. TCP MSS clamping (`--clamp-mss-to-pmtu`) on NetBird-routed traffic (mangle table, Hashi chain).
10. Persist with `netfilter-persistent save` where available (Debian/Ubuntu), OpenRC save on Alpine.

IPv6: same structure with `ip6tables` when the host has public IPv6; DNAT for IPv6 only if configured.

### 3.2 Taking over from an existing firewall script

If the host already runs its own firewall script, removing it is part of Hashi's **first apply plan** (shown explicitly,
executed under the rescue timer): its chains (e.g. `CUSTOM_DNAT`, `CUSTOM_FWD`, `CUSTOM_INPUT`), their jump rules, its
ipsets and its cron/systemd entries. Rules owned by NetBird, Docker or Proxmox are never part of this cleanup.

## 4. NetBird compatibility

All firewall hosts run NetBird; it is often the rescue path. NetBird support is first-class, not legacy.

Detection: `netbird` installed and service active; `netbird status` output; interface (default `wt0`, overridable);
overlay CIDRs (detected or entered, e.g. `100.110.0.0/16`); routed-network CIDRs. Optional NetBird management API
connection for display/validation of peers, groups and network resources; without it, SSH detection + user CIDRs suffice.

Modes: **peer only** (allow NetBird access to the host), **routing peer** (also forward NetBird ↔ managed subnets with
masquerade + MSS clamp), **disabled** (no NetBird-specific Hashi rules, NetBird untouched).

Preserve the current script's behaviours: configurable interface, overlay source allowed on the NetBird interface
before final drop, atomic trusted-IP refresh, public-port DNAT to Traefik, hairpin NAT, managed-subnet masquerade, `netfilter-persistent save`.

Safety: never flush NetBird's tables/chains/nftables rules; never assume `wt0`; never require public inbound ports for
NetBird; warn before any change that could remove NetBird access; rescue protection on every high-risk apply (§5).

## 5. Rescue rollback (first apply and every high-risk apply)

1. Save current state (`iptables-save`, `ip6tables-save`, `ipset save`) to `previous.rules`.
2. Write `rollback.sh` with mode 0700 **before** arming.
3. Arm a transient timer: `systemd-run --on-active=180 --unit=hashi-fw-rescue /opt/hashi/firewall/rollback.sh`
   (Alpine: `at` or a backgrounded `sleep 180 && rollback` with a PID file).
4. Apply the new rules.
5. Hashi opens a **new** SSH connection (not the existing one) over the same address it uses as the host's rescue path,
   verifies it can log in and run a command, and — if NetBird is enabled — verifies NetBird status is still connected.
   Only then disarm the timer.
6. If verification fails or times out, do nothing: the timer restores the previous state. Hashi reconnects after the
   window, reads the actual ruleset and reports the rollback by checking the remote result, not assuming it.
7. After a rollback, Hashi compares the non-Hashi rules (NetBird, Docker, others) with the snapshot from step 1 and the
   state read just before the rollback. If a foreign rule changed during the window, the rollback restored the old
   version; Hashi shows a warning naming those rules and does nothing else.

The full restore in rollback is the one deliberate exception to "owned objects only" (invariant 12): it only runs when an
apply locked Hashi out, foreign rules on these hosts change rarely (NetBird on peer/policy changes; Docker runs in
guests, not on the hosts), so a collision inside the 180 s window is very unlikely and is surfaced by step 7 rather than
prevented by a more complex partial restore.

## 6. Desired ports and allowlists

- Ports come from resources (80/443 always when any HTTP-family resource exists; stream ports per resource).
- Opening or closing a port is high-risk. Port conflicts with services already listening on the host are detected (`ss -lntup`) and shown.
- Allowlist entries: IP/CIDR with reason, expiry; managed under Firewall → Allowlist.
- Firewall block entries (06-edge-security.md §8): applied to `hashi_blocked[6]` on all hosts; per-host applied state,
  last result/error, last hit (from `ipset` counters where enabled).

## 7. API

`/api/firewall/hosts` CRUD, `POST /api/firewall/hosts/{id}/validate|discover|plan`, `GET /api/firewall/hosts/{id}/ruleset`
(observed), `GET /api/firewall/hosts/{id}/rendered`, `/api/firewall/allowlist` CRUD, `POST /api/firewall/hosts/{id}/rollback` (reauth + `firewall.apply`).

## 8. Tests that must exist

- shellcheck on every golden rendered script.
- Integration in a privileged test container (or netns): apply on a ruleset containing foreign chains + NetBird-like
  chains → foreign and NetBird rules byte-identical afterwards; Hashi chains present in the right tables.
- DNAT test: packet to public port reaches Traefik IP:entrypoint (netns topology).
- Rescue: simulate verification failure → previous ruleset restored by the timer.
- Pulse IPs matching managed subnets produce CNAMEs, not A records (with 03-dns.md).
