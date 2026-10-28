# ADR 0002 — Security model: IP sets, rule sets, two-stage edge engine, nftables

Date: 2026-10-27 · Status: accepted (signed off by the user 2026-10-28)

## Context

The first spec (from V2) described security as several overlapping mechanisms: manual allow/block entries with five
scopes and five bypass flags, security subjects with eight states, soft blocks vs firewall blocks, blocklists with their
own enforcement modes, per-resource policy modes, resource rules with their own match types, and a separate
"connection target" model for Pulse-bound addresses. Each place that needed an address had its own way to express one,
so things that obviously belonged together (a Pulse-tracked home IP in an allowlist, as a DNS value, as an SSH target)
could not be combined. None of it was built yet (S1 is the last finished slice).

The user designed a replacement model (input archived in `docs/archive/2026-10-27-security-model-source.md`). This ADR
records what was adopted, what was corrected and why, and the decisions taken with the user.

## Decision

1. **IP sets are the only address primitive** (05a-ip-sets.md). An IP set holds items (IP, CIDR, Pulse agent reference,
   host reference, DNS name, country/region/ASN) and other IP sets (acyclic). Every place in Hashi that needs "who" or
   "where" takes an IP set: firewall allow/block, host access, rule `Applies To` (match positions), and DNS record values,
   service upstreams, connection/SSH targets (target positions, address-only sets). Feeds and the engine's dynamic state
   are IP sets too.
2. **Rule sets are the only L7 policy primitive** (06-edge-security.md §4). Rule items (`Applies To` IP set, optional path
   condition, action `None` / `Deny` / `Pass CAPTCHA` / `Pass Authentication`) and nested rule sets with unique priorities,
   flattened pre-order. Every HTTP service owns a root rule set; then the global rule set; then the service's default
   fallback (`None` or `Deny`). First match wins.
3. **Two stages for every HTTP request**: Stage A (global abuse engine: block guard, WAF and rate trigger, challenge
   enforcement with escalation to a ban) always runs before Stage B (the service's flattened rules).
4. **Settings vs rules**: thresholds, timers and the ban function `T_ban(c)` are global settings; per service only the
   rule set, the default fallback and WAF mode per path are configurable.
5. **Firewall folding**: combined manual allow and block are resolved by longest prefix match (tie → block); manual
   allow outranks every dynamic ban. The kernel only receives the folded result.
6. **nftables, Hashi's own table** (`table inet hashi`) instead of iptables/ipset chains. Kernel meters add violators to
   a staging set inline; Hashi drains it over SSH and turns it into bans.
7. **"Resource" is renamed "Service"** (HTTP-family = L7 service, TCP/UDP = L4 service).

Decided with the user while turning the draft into spec:

- Staging set drain and ban push run over a **persistent SSH connection per firewall host** ("host link"), not a host
  agent (the draft's "Hashi Host Agent"). Nothing is installed on the host; Hashi keeps its SSH client connection open
  and runs short `nft` commands over it. Pulse stays heartbeat-only and credential-free. See "Host link vs host agent".
- **No observe/dry-run mode and no WAF detect-only mode** (confirmed again after discussing a per-path `detect` mode): a
  false positive shows as a broken app; the fix is a path exclusion, a WAF mode per path, or resetting the ban and counter.
- The **WAF stays a Traefik plugin** (Coraza + OWASP CRS, pinned). Hashi tails Traefik's access log continuously (seconds,
  not the former 60 s batch) to count WAF hits per address.
- **Exempt from the L7 rate trigger**: addresses in the effective manual allow, and requests carrying a valid edge SSO
  session. The WAF still rejects their malicious requests.

## Corrections to the draft (and why)

| Draft | Problem | Spec |
| --- | --- | --- |
| Meters in `prerouting priority raw` | Conntrack runs at −200; at raw (−300) `ct state` / `ct count` have nothing to read | Block drops stay at raw; meters run in a second prerouting chain at −150 (after conntrack, before DNAT) |
| `protocol tcp ct count over 250` | Without a per-source set this counts all connections together; connection #251 of anyone gets banned | `add @m_tcp_conc { ip saddr ct count over C }` |
| Meters on all prerouting traffic | Would count LAN/LXC egress and NetBird traffic (e.g. the Docker host pulling images gets banned) | Meters only on the WAN interface and only for published ports |
| "Hashi skips manually allowed IPs" when draining staging | The kernel already dropped them for the staging timeout | The effective allow set is in the kernel and is checked before staging, dynamic and meter rules |
| UDP packet-rate meter on all UDP | For bulk transfer pps ∝ bandwidth; 5000 pps ≈ 55 Mbit/s of QUIC, so an HTTP/3 download gets banned | UDP meters only on published L4 UDP stream ports; 443 is guarded by Stage A |
| IPv4 only | Services may be published over IPv6; a /128 ban is useless | Parallel v6 sets; dynamic v6 entries cover a /64 |
| One `interval, timeout, auto-merge` block set | Merging neighbours with different timeouts is fragile; rebuilt static content and engine bans have different lifecycles | Static folded block set (interval, auto-merge, replaced atomically) + dynamic block set (timeouts) |
| Block drops on everything from the source | A country or feed block could contain the network the admin's phone uses and kill NetBird, the rescue path | NetBird's WireGuard port is exempt from Hashi drops; admin sources belong in the allow set (also exempt) |
| `nft -f /etc/nftables-hashi.conf` | Debian's `nftables.service` loads `/etc/nftables.conf`, which starts with `flush ruleset` and would wipe NetBird/Proxmox/Docker state at boot | Hashi's own unit loads only `table inet hashi`; validation warns if `nftables.service` is enabled |
| `Deny` = 403 "or connection drop" | Forward auth can only answer with a status | `403` |
| Engine bans pushed instantly | Invariant 7 required a plan for every firewall change | Invariant 7 amended: engine-made dynamic set elements are security events, not plans; structure and manual sets stay plan-gated |

## Gaps filled from the existing spec

- Host access (SSH, Proxmox UI on the firewall host itself): per host, IP sets allowed to reach the host; default the
  system set `hashi-hosts`; plus loopback, managed subnets and NetBird as before.
- Item types `host` (firewall/edge host addresses detected over SSH) and `dns` (FQDN resolved by Hashi; replaces the
  former "trusted FQDN" refresh). System sets `hashi-hosts` and `private-networks`.
- Items carry an optional note and expiry (former allowlist entries had reason + expiry).
- `Pass Authentication` takes an optional provider and optional allowed groups (former per-resource SSO options).
- `Pass CAPTCHA` clearance cookie lifetime: setting `captcha.clearance_ttl`, default 24 h. A valid edge SSO session
  also satisfies `Pass CAPTCHA` (a logged-in user on a VPN or data-center address is not asked for a CAPTCHA).
- Forward-auth outage: services whose flattened rules can deny or require authentication fail closed; the rest fail open.
- Target resolution that yields no address never empties applied state (DNS value, upstream, SSH target).
- Two persistent per-address counters: the ban counter (drives `T_ban`) and an informational challenge counter, both
  visible, filterable, editable and resettable (Security → Counters, address page). Both use **step decay**: −1 per
  `ban.counter_decay_ttl` of quiet time, the clock starting at the end of the last ban (last challenge), computed at
  read time. Reason: addresses are reassigned (dynamic home IPs, CGNAT, cloud), so history must fade, but a heavy
  offender fades slower than a one-time one. The draft had no decay; the first spec had a full reset after a window.
- WAF rule exclusions per service/path are kept (CRS false positives would otherwise force `Disabled`).

## Host link vs host agent

| | Persistent SSH (chosen) | Host agent |
| --- | --- | --- |
| On the host | nothing new; SSH is already required for validation, plans and rescue | a root-capable binary per firewall host: install, update, own credentials, own release |
| Ban latency | ≤ `l4.poll_interval` (3 s); HTTP is blocked at once by A1 | instant |
| Hashi down | kernel keeps enforcing `block`, `dyn`, staging and meters; no new bans (the engine is Hashi) | same: the engine is still Hashi |
| Cost | reconnect with backoff; 5-min reconciliation repairs anything missed | a second channel next to SSH, which plans still need |

SSH wins for a few firewall hosts that Hashi already reaches. Revisit when a firewall host is not reachable by SSH from
Hashi, when there are many firewall hosts, or when enforcement must make decisions without Hashi. Such an agent would be
its own program with its own name, never part of Pulse: Pulse must stay small enough to run dozens of times per host (one
per Compose stack). A host agent may embed Pulse's heartbeat; Pulse never embeds the host agent.

## Rejected alternatives

- **Pulse as a privileged host agent** on firewall hosts: would turn the credential-free heartbeat agent into a
  root-capable one. SSH is already trusted with root on those hosts.
- **Coraza inside Hashi** (forward auth with `forwardBody`): one engine and instant counts, but every inspected body
  would cross Hashi with a size limit that breaks large uploads.
- **iptables-legacy + ipset** with the same model (`hashlimit` ≈ meter, `connlimit` ≈ `ct count`, `SET --add-set` ≈
  `add @set`, ipset timeouts, `nomatch` for carve-outs): matches what the first firewall host runs today (iptables-legacy;
  NetBird uses legacy chains and ipsets there). Rejected in favour of nftables because Hashi gets one table it fully owns
  (narrower rollback, JSON in and out, atomic transactions), **unless** the S6 coexistence gate fails — then this is plan B
  and the model above stays unchanged.

## Consequences

- One compiler (IP set → prefix lists + geo predicates) feeds the firewall, forward auth, DNS, Traefik upstreams and
  connections. Pulse IP changes, DNS refreshes, feed updates and engine bans all flow through it.
- Rescue rollback restores only `table inet hashi`, except for the first takeover apply, which also removes the old
  script's iptables-legacy chains and ipsets.
- S6 starts with a coexistence capture on the real host (kernel, nft 1.1.x, iptables-legacy NetBird): nft syntax checked
  with `nft -c`, legacy NAT + nft NAT side by side in a netns test, and whether NetBird changes its firewall backend after
  a restart when a foreign nftables table exists.
- Former tables `security_subjects`, `security_subject_states`, `manual_security_entries`, `ban_policies`,
  `offense_counters`, `blocklist_*`, `firewall_allow_subjects`, `firewall_block_applied`, `connection_targets` and
  `resource_rules` are not built; 06a-security-data-model.md defines their replacements.
- Approved S0b mocks for Security, Subject, Firewall and the resource Rules tab no longer match; they are redone after
  sign-off, together with the new IP set, rule set and address pages.
