# 05 — Linux firewall hosts

A Linux firewall host is a machine with a public IP (typically a Proxmox host) that forwards public ports into managed
subnets where Traefik runs. Hashi owns one nftables table on it (`table inet hashi`), enforces the folded allow/block
result of the global IP set bindings, runs the L4 kernel meters, keeps NetBird working, and protects the rescue path.
Addresses always come from IP sets ([05a-ip-sets.md](05a-ip-sets.md)). Decision record: [ADR 0002](../adr/0002-security-model.md).

**Changed from the first spec:** iptables chains + ipsets → one nftables table; per-host allowlist entries and firewall
block entries → global allow/block IP set bindings folded by longest prefix match; kernel meters and a staging set added;
rescue rollback scoped to Hashi's table.

## 1. Host model

Name (→ public FQDN `name.<root>`), public IP (detected or override), public IPv6 (optional), managed subnets (multiple
CIDRs), linked Traefik connection, internal Traefik target (address set, 05a §5.2), route target override (`on.` record,
03-dns.md §5.1), SSH target (address set) + credentials + pinned host key, OS family, WAN interface (detected or
override), LXC bridge (detected or override), NetBird settings (§5), host access (list of IP sets, §3.2).

Every registered host appears in the system IP set `hashi-hosts` (05a §3), which is part of the combined allow (§4.1).

### 1.1 Edge modes

| Mode | When | Hashi manages |
| --- | --- | --- |
| `managed` | Host owns its public IP (e.g. a VPS) | Everything in this file |
| `external_nat` | Host sits behind a router/NAT it doesn't control (e.g. home) | No firewall artifacts. Host records still exist (public IP detected from the host's egress, from a Pulse agent, or set manually); it is in `hashi-hosts`; the UI shows which ports must be forwarded on the router; the typical setup routes its `on.` record via a `managed` host. |

## 2. Setup and validation

Collect: name, SSH target and credentials (same flow as Traefik), managed subnets, linked Traefik connection, internal
Traefik target, optional WAN interface / bridge / public IP overrides.

Validate: SSH login + host-key pin; OS detection; kernel version; `nft` present and its version (≥ 1.0 required, JSON
support checked with `nft -j list tables`); which iptables backend is active (`iptables -V` → `legacy` or `nf_tables`)
and which firewall backend NetBird uses (its chains/sets in iptables-legacy or its nftables tables); whether Debian's
`nftables.service` is enabled (warning: its `/etc/nftables.conf` normally starts with `flush ruleset` and would wipe
NetBird, Proxmox and Docker rules at boot — Hashi never uses it); public IP detection (from the host, cross-checked with
Hashi's view); managed subnets valid, non-overlapping, containing the internal Traefik IP; NetBird detection (§5).

**Read the existing ruleset first.** Setup captures `nft -j list ruleset`, `iptables-save`, `ip6tables-save` (and the
`-legacy` variants where present) and `ipset save`, and shows them; Hashi's first plan is computed relative to this real state.

## 3. The Hashi table

### 3.1 Artifacts on the host

| Path | Purpose |
| --- | --- |
| `/opt/hashi/firewall/hashi.nft.json` | Rendered table definition (libnftables JSON, marshalled from Go structs) |
| `/opt/hashi/firewall/sysctl` → `/etc/sysctl.d/90-hashi.conf` | `net.ipv4.ip_forward=1` (and IPv6 forwarding when configured) |
| `/opt/hashi/firewall/previous.nft.json` + `rollback.sh` | Rescue restore (mode 0700, §6) |
| `hashi-firewall.service` (systemd) / OpenRC service | Loads `hashi.nft.json` at boot after network-online; never `flush ruleset` |

Packages ensured by the apply plan: Debian/Ubuntu `nftables` (the binary; the `nftables.service` unit stays disabled),
Alpine `nftables`. Persistence is the boot unit plus Hashi re-pushing dynamic entries when the host link reconnects (§6).

### 3.2 Contents

All names below live inside `table inet hashi`; every set exists as a `…4` (`ipv4_addr`) and `…6` (`ipv6_addr`) pair.

Sets:

| Set | Flags | Content | Written by |
| --- | --- | --- | --- |
| `allow` | interval, auto-merge | effective allowed ranges from folding (§4) | plan (atomic replace) |
| `block` | interval, auto-merge | effective statically blocked ranges from folding | plan (atomic replace) |
| `dyn` | timeout (v6: interval, timeout, entries are /64) | engine bans from `dynamic-blocked`, minus allowed ranges | host link (§6) |
| `staging` | dynamic, timeout (default `l4.staging_timeout` = 5 m) | L4 meter violators, added by the kernel | kernel; drained by host link |
| `host_access` | interval, auto-merge | resolved host access sets of this host | plan |
| `pub_tcp`, `pub_udp` | — (`inet_service`) | published public ports | plan |
| `metered_udp` | — | published L4 UDP stream ports (never 443/udp) | plan |
| `m_tcp_new`, `m_tcp_conc`, `m_udp_new`, `m_udp_pps` | dynamic, timeout | per-source meter state | kernel |

Chains (hook · priority):

1. `pre_guard` · prerouting · raw (−300). Only `iifname` = WAN: the NetBird WireGuard listen port → `return`;
   `@allow` → `return`; `@block` → drop; `@dyn` → drop; `@staging` → drop.
2. `pre_meter` · prerouting · −150 (after conntrack, before DNAT). Only WAN, only `@pub_tcp` / `@metered_udp`, not
   `@allow`:
   - TCP new-connection rate: `tcp flags syn ct state new update @m_tcp_new { ip saddr limit rate over R_tcp/minute burst B_tcp packets } add @staging { ip saddr } drop`
   - TCP concurrency: `ct state new add @m_tcp_conc { ip saddr ct count over C_tcp_max } add @staging { ip saddr } drop`
   - UDP new-flow rate (`@metered_udp` only): `ct state new update @m_udp_new { ip saddr limit rate over R_udp_flow/minute burst … } add @staging { ip saddr } drop`
   - UDP packet rate (`@metered_udp` only): `update @m_udp_pps { ip saddr limit rate over R_udp_pps/second burst … } add @staging { ip saddr } drop`
   The exact syntax for the host's nft version is captured and checked with `nft -c` before it is used (§9, S6 gate).
3. `nat_pre` · nat prerouting · dstnat: DNAT each published TCP/UDP port to the linked internal Traefik IP **and the
   Traefik entry-point port** for that service (80/443 for HTTP family, the stream entry point for TCP/UDP — not the
   upstream service port).
4. `input` · filter input · filter: accept loopback, established/related, managed subnets, NetBird interface, the
   NetBird WireGuard port, `@host_access`; final drop for other new inbound traffic only after the user accepted the
   "block all other inbound" warning for this host.
5. `forward` · filter forward · filter: accept DNATed flows (`ct status dnat`), managed-subnet egress, established/related.
6. `nat_post` · nat postrouting · srcnat: masquerade managed subnets → WAN; hairpin NAT so internal clients reach public
   endpoints; NetBird routing-peer masquerade where Hashi owns it (§5).
7. `mangle_fwd` · filter forward · mangle: TCP MSS clamp on NetBird-routed traffic.

A drop in Hashi's table is final; an accept or return only ends Hashi's chain — other tables (NetBird, Proxmox, Docker)
still see the packet. Hashi relies on that: it never needs to touch their rules.

L4 published ports are public by design. A service that must be reachable only privately is not published; it is
reached over NetBird. The meters measure connection rates, concurrency and packet rates — never bandwidth (legitimate
downloads saturate bandwidth; botnets stay below byte limits). HTTP/3 on 443/udp is excluded from UDP meters because for
bulk transfers packet rate grows with bandwidth; HTTP traffic is guarded by Stage A (06 §3).

### 3.3 Applying a structural change

The table definition is applied in one `nft -j -f` transaction: create the table if missing; declare sets (an existing
set with the same definition is kept with its elements); flush and re-add every chain; atomically replace the elements
of `allow`, `block`, `host_access`, `pub_*`, `metered_udp` (`flush set` + `add element` in the same transaction).
`dyn`, `staging` and meter sets are never flushed by an apply. If a dynamic set's definition must change, the same
transaction deletes and re-creates it and re-adds the current `dynamic-blocked` entries with their remaining time.

Threshold changes (`l4.*` settings) re-render only `pre_meter`; existing bans and staging entries stay.

### 3.4 Taking over from an existing firewall script

If the host already runs its own firewall script, removing it is part of Hashi's **first apply plan** (shown explicitly,
executed under the rescue timer): its iptables chains (e.g. `CUSTOM_DNAT`, `CUSTOM_FWD`, `CUSTOM_INPUT`), their jump
rules, its ipsets and its cron/systemd entries. Rules and sets owned by NetBird, Docker or Proxmox are never part of
this cleanup, in any backend.

## 4. Folding: global allow and block

### 4.1 Inputs

- **Combined manual allow** `S_allow`: the union of the IP sets bound to *Global firewall allow* plus `hashi-hosts`
  (so `external_nat` hosts such as the home server are never blocked).
- **Combined manual block** `S_block`: the union of the IP sets bound to *Global firewall block* (feed sets are bound here
  to be enforced at the firewall, 06 §8).
- **Dynamic blocked** `S_dyn`: entries of `dynamic-blocked` (06 §3.4).

Bindings are global: every `managed` host receives the same folded result. The bindings live on the Firewall page.

### 4.2 Algorithm

For every address `x`: let `a` be the longest prefix in `S_allow` containing `x`, `b` the longest in `S_block`.

| `a` | `b` | Verdict |
| --- | --- | --- |
| — | — | neutral (no Hashi decision) |
| yes | — | allowed |
| — | yes | blocked |
| yes | yes | allowed if `len(a) > len(b)`, otherwise blocked (tie → block, fail closed) |

Computed by a sweep over the sorted interval endpoints of both compiled lists, per address family; the output is a
partition into allowed, blocked and neutral ranges. Kernel `allow` = allowed ranges; kernel `block` = blocked ranges.

**Manual beats dynamic:** an address in an allowed range is never added to `dyn` (Hashi refuses the entry, the engine
never creates it, 06 §3), and `pre_guard` checks `@allow` before `@dyn` and `@staging`, so even the kernel's own staging
drop never hits an allowed address (invariant 20). A dynamic ban inside a statically blocked range is still recorded
(ban counter, events).

The same compiled partition is used by Stage A1 in forward auth (06 §3.1), so the firewall and the edge agree.

### 4.3 Explaining a verdict

The address page (05a §6) shows, per host, the verdict, the deciding prefix and the set (and nesting path) it came from.
The plan preview shows, per set bound, how many ranges it contributes, how many were carved out by the other side, and
the warning "allow covers more than a /24 (IPv4) / /48 (IPv6) or a whole country/ASN" where it applies.

### 4.4 Risk

| Change | Risk |
| --- | --- |
| `allow` / `block` / `host_access` element updates | low (auto-apply), except: |
| — an allowed or host-access range that contains the current admin session's address or the host's rescue source is removed | high |
| — a newly blocked range of /16 or larger (IPv6 /32) | high |
| Chain/structure change, port open/close, first apply | high, under the rescue timer |
| Meter threshold change | low on hosts whose rescue path is not NetBird, else high |
| Dynamic entries (engine bans, unbans, staging drain) | not a plan: security events via the host link (invariant 7) |

## 5. NetBird compatibility

All firewall hosts run NetBird; it is often the rescue path. NetBird support is first-class.

Detection: `netbird` installed and service active; `netbird status` output; interface (default `wt0`, overridable);
WireGuard listen port; overlay CIDRs (detected or entered, e.g. `100.110.0.0/16`); routed-network CIDRs; NetBird's
firewall backend (iptables-legacy chains/ipsets or its own nftables tables). Optional NetBird management API connection
for display/validation of peers, groups and network resources; without it, SSH detection + user CIDRs suffice.

Modes: **peer only** (allow NetBird access to the host), **routing peer** (also forward NetBird ↔ managed subnets with
masquerade + MSS clamp), **disabled** (no NetBird-specific Hashi rules, NetBird untouched).

Safety: never flush or edit NetBird's tables, chains or sets in any backend; never assume `wt0`; never require public
inbound ports for NetBird; the WireGuard listen port and the NetBird interface are exempt from Hashi drops; warn before
any change that could remove NetBird access; rescue protection on every high-risk apply (§7).

## 6. Host link (dynamic entries)

Hashi keeps one persistent SSH connection per `managed` firewall host (pinned host key, same credentials). Over it:

1. Every `l4.poll_interval` (default 3 s): `nft -j list set inet hashi staging4` and `staging6`.
2. For each staged address: skip if allowed (should not happen, §4.2); otherwise hand it to the engine (06 §3.4), which
   increments the ban counter, computes `T_ban(c)` and creates a `dynamic-blocked` entry.
3. Push all pending dynamic changes as one `nft -j -f -` transaction per tick and host: `add element … dyn { a timeout T }`
   (no timeout for permanent bans), `delete element … staging { a }`, `delete element … dyn { a }` for unbans.
4. Record per-host applied state for every entry (pending / applied / failed + error).
5. Every 5 min and on every reconnect: list `dyn` on the host, diff with the database, re-add missing entries with their
   remaining time, delete entries Hashi does not know.

If the link is down, the kernel keeps enforcing: staging entries drop traffic until their timeout, `dyn` entries until
theirs. Staging entries that time out while Hashi cannot drain them are lost (no ban counter increment) — accepted.
Host link state (connected, last poll, last push, lag) is shown on the firewall host page and is a job (invariant 15).

## 7. Rescue rollback (first apply and every high-risk apply)

1. Save Hashi's current table: `nft -j list table inet hashi` → `previous.nft.json` (none on the first apply). The first
   apply also saves `iptables-save`/`iptables-legacy-save`, `ip6tables-save` and `ipset save` of the takeover objects (§3.4).
2. Write `rollback.sh` with mode 0700 **before** arming: delete `table inet hashi`, load `previous.nft.json` if present;
   on the first apply also restore the saved takeover objects.
3. Arm a transient timer: `systemd-run --on-active=180 --unit=hashi-fw-rescue /opt/hashi/firewall/rollback.sh`
   (Alpine: `at` or a backgrounded `sleep 180 && rollback` with a PID file).
4. Apply.
5. Hashi opens a **new** SSH connection (not the existing one) over the address it uses as the host's rescue path,
   verifies it can log in and run a command, and — if NetBird is enabled — verifies NetBird status is still connected.
   Only then disarm the timer.
6. If verification fails or times out, do nothing: the timer restores the previous state. Hashi reconnects after the
   window, reads the actual ruleset, re-pushes dynamic entries, and reports the rollback by checking the remote result.

Rollback only touches Hashi's own table. The single exception is the first takeover apply, whose rollback also restores
the old script's objects it removed; foreign rules (NetBird, Docker, Proxmox) are never part of either.

## 8. Ports

- Ports come from services (80/443 always when any HTTP-family service exists, plus 443/udp when HTTP/3 is enabled;
  stream ports per L4 service).
- Opening or closing a port is high-risk. Port conflicts with services already listening on the host are detected
  (`ss -lntup`) and shown.

## 9. API

`/api/firewall/hosts` CRUD, `POST /api/firewall/hosts/{id}/validate|discover|plan`, `GET /api/firewall/hosts/{id}/ruleset`
(observed, all tables, Hashi's highlighted), `GET /api/firewall/hosts/{id}/rendered`, `GET /api/firewall/hosts/{id}/kernel-sets`
(sizes, staging content, host link state), `GET|PUT /api/firewall/bindings` (global allow/block set lists),
`GET|PUT /api/firewall/hosts/{id}/host-access`, `POST /api/firewall/hosts/{id}/rollback` (reauth + `firewall.apply`).

## 10. Tests that must exist

- Golden rendered JSON per host shape (managed with/without IPv6, NetBird modes, no ports, many ports); every rendered
  table passes `nft -c -j -f` in the integration container.
- Folding: table-driven over the §4.2 cases (nested prefixes, tie, allow inside block inside allow, IPv6), property test
  that the output is a partition and equals a brute-force verdict on random addresses.
- `TestInvariant_20_ManualAllowBeatsDynamic`: an allowed address is never in `dyn`, staging is bypassed for it in the
  kernel (netns test), and L7 never bans it.
- Integration in a privileged netns with iptables-legacy NetBird-like chains + ipsets and a foreign nft table: apply →
  foreign rules byte-identical; Hashi's table present; legacy MASQUERADE and Hashi's DNAT both work for their flows.
- DNAT test: packet to a public port reaches Traefik IP:entrypoint.
- Meters: a test source exceeding each meter lands in `staging` and is dropped; an allowed source exceeding it is not;
  LAN-side traffic is never metered.
- Structural re-apply keeps `dyn` and `staging` elements; threshold change keeps them.
- Host link: drain → ban → push; reconnect reconciliation re-adds missing entries with remaining time.
- Rescue: simulate verification failure → previous table restored by the timer, foreign rules untouched.

**S6 entry gate (before any renderer code):** on the real host, capture kernel and nft versions, NetBird's backend, and
check every rule shape of §3.2 with `nft -c` (check mode, no commit — user OK required); record in the netns test
whether NetBird changes its firewall backend after a restart when a foreign nftables table exists. If coexistence fails,
switch to plan B in ADR 0002 (iptables-legacy + ipset implementing the same model) and record it as a spec change.
