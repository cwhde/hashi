# Security model — original input (archived 2026-10-27)

> **Archive. Not a specification.** Coding agents: do not read or implement from this file. The authoritative version
> of everything below lives in `docs/spec/` (05-firewall.md, 05a-ip-sets.md, 06-edge-security.md, 06a-security-data-model.md)
> and `docs/adr/0002-security-model.md`. Where this text and the spec differ, the spec wins and the ADR says why.
>
> Kept for history: the user's design as written (real hostnames replaced by `edge-a`, `edge-b`, `example.com` because
> the repository is public), preceded by the decisions taken while turning it into spec.

## Decisions taken in the same conversation

1. Hashi reads the kernel staging set and pushes bans over a persistent SSH connection per firewall host (no host agent;
   Pulse stays heartbeat-only).
2. The WAF stays a Traefik plugin (Coraza + OWASP CRS); Hashi tails the access log continuously to count WAF hits.
3. Exempt from the global L7 rate trigger: addresses in the combined manual allow set and requests with a valid edge SSO
   session. The WAF still applies to them.
4. No observe / dry-run mode and no WAF detect-only mode. Enforce from the start.
5. IP sets are the one address primitive everywhere: not only for matching (allow/block lists, rule `Applies To`) but also
   as targets (DNS record values, service upstreams, connection/SSH targets), so anything can be "set via Pulse".
6. "Resource" is renamed to "Service" (L7 and L4).

Corrections to the text below (reasons in the ADR): meters cannot run at `priority raw` (no conntrack yet); `ct count`
needs a per-source set; meters only on the WAN interface and published ports; manual allow must be in the kernel before
the meters and staging drop; UDP packet-rate meter not on 443 (HTTP/3); IPv6 sets and /64 bans; static and dynamic
block sets split; NetBird/rescue path exempt from block drops; never use Debian's `nftables.service` (`flush ruleset`).

---

### Part 1: Verification — Does `nftables` Make Dynamic IP Sets & Syncing Easy?

**Yes — it is a first-class, built-in feature of `nftables`, and parsing/manipulating it from C# / Python / Go is trivial because `nftables` has a native `-j` (JSON) flag.**

Here is the exact low-level mechanics of how `nftables` handles this in 4 lines of config and 3 shell commands:

#### 1. How `nftables` Automatically Adds Violating IPs to a Set

In `nftables`, any set declared with `flags dynamic` can be mutated directly inside a packet-matching rule using the `add @set_name { ip saddr }` statement. You don't need any external daemon to catch the packet; the Linux kernel does it inline at wire speed:

```nft
table inet hashi {
    # 1. Staging queue where the kernel dumps L4 rate-limit violators
    set hashi_l4_staging {
        type ipv4_addr
        size 65535
        flags dynamic, timeout
        timeout 5m
    }

    # 2. The main folded blocklist managed by Hashi
    set hashi_folded_block {
        type ipv4_addr
        flags interval, timeout
        auto-merge
    }

    chain prerouting {
        type filter hook prerouting priority raw; policy accept;

        # Drop anything in the main folded blocklist OR the L4 staging set immediately
        ip saddr @hashi_folded_block drop
        ip saddr @hashi_l4_staging drop

        # TCP New Connection Rate Meter -> Violators go straight into @hashi_l4_staging
        tcp flags syn ct state new meter tcp_syn_meter { ip saddr limit rate over 150/minute burst 30 packets } add @hashi_l4_staging { ip saddr } drop

        # TCP Concurrent Connections Meter -> Violators go straight into @hashi_l4_staging
        protocol tcp ct count over 250 add @hashi_l4_staging { ip saddr } drop

        # UDP New Flow Rate Meter -> Violators go straight into @hashi_l4_staging
         meta l4proto udp ct state new meter udp_flow_meter { ip saddr limit rate over 200/minute burst 50 packets } add @hashi_l4_staging { ip saddr } drop

        # UDP Raw Packet Flood (PPS) Meter -> Violators go straight into @hashi_l4_staging
        meta l4proto udp meter udp_pps_meter { ip saddr limit rate over 5000/second burst 1000 packets } add @hashi_l4_staging { ip saddr } drop
    }
}

```

Notice how `@hashi_l4_staging` also drops packets immediately (`ip saddr @hashi_l4_staging drop`). That means the microsecond an IP breaches the L4 rate limit, the kernel blocks it on the spot without waiting for Hashi's polling loop.

#### 2. How Hashi Parses, Removes, and Moves the IP to the Global Set

Because `nft` supports native JSON output (`nft -j`), Hashi never has to regex-parse text output:

1. **Read new entries (JSON):**
`nft -j list set inet hashi hashi_l4_staging`
Returns a clean JSON array of every offending IP currently sitting in the staging set.
2. **Process in Hashi:**
For each IP found, Hashi increments the stored Ban Counter $c(\text{IP}) \leftarrow c(\text{IP}) + 1$, calculates $T_{\text{ban}}(c)$, and adds the IP to Hashi's central `Dynamic Blocked IP Set`.
3. **Move from Staging Set $\to$ Main Folded Block Set (Atomic Transaction):**
Hashi executes a single atomic command (or passes `-f -` via stdin):
```sh
nft "add element inet hashi hashi_folded_block { 198.51.100.44 timeout 3600s }; delete element inet hashi hashi_l4_staging { 198.51.100.44 }"

```


4. **Updating Thresholds Synchronized Through Hashi:**
When you change `R_tcp` or `R_udp` in Hashi's UI settings, Hashi simply re-renders the `chain prerouting` block and runs `nft -f /etc/nftables-hashi.conf`, which atomically replaces the meter thresholds in the kernel without flushing your existing IP sets.

Because all three requirements (auto-add on violation, JSON parsing, and atomic move between sets) are natively built into `nftables`, we lock this in and write out the complete, exhaustive specification below.

---

---

# HASHI SECURITY ENGINE — COMPLETE IMPLEMENTATION & DESIGN SPECIFICATION

## 1. Architectural Overview & Design Philosophy

The Hashi Security Engine governs network filtering (Layer 3/4) and application access control & abuse mitigation (Layer 7) across a distributed topology of public VPS ingress nodes and private/NATed backend hosts connected via mesh VPN (NetBird).

### Core Design Tenets

1. **Strict Separation of Rules vs. Settings:**
* **Rules** are deterministic routing/access mappings (*Who* matches *What condition* $\to$ *Which Action*).
* **Settings** are global system parameters, rate thresholds, timers, and mathematical state functions that govern the automated abuse engine.


2. **Two Parallel Recursive Primitives (`IP Sets` & `Rule Sets`):**
* Everything describing *Who* is an `IP Item` or `IP Set` (which can recursively contain other `IP Sets`).
* Everything describing *L7 Access Policy* is a `Rule Item` or `Rule Set` (which can recursively contain other `Rule Sets`), where a `Service` is itself a root `Rule Set`.


3. **Zero Ambiguity in Precedence:**
* On Layer 3/4 (`nftables`), conflicts between manual sets are resolved deterministically by **Longest Prefix Match (most specific CIDR)**, and conscious manual entries always override automated dynamic bans.
* On Layer 7, the **Global Abuse Engine (Stage A)** always executes before **Custom Access Rules (Stage B)**. Within Stage B, recursive `Rule Sets` flatten via explicit integer priorities into a single top-to-bottom list where the first matching rule wins.


4. **Scoped vs. Global State Isolation:**
* Custom service rules requiring CAPTCHA or Authentication only gate that specific service/path; they never pollute the global dynamic ban or challenge sets.
* Conversely, abusive behavior (WAF triggers, HTTP request floods, CAPTCHA brute-forcing, or L4 connection floods) is tracked globally and escalates to a synchronized network-wide firewall ban.



---

## 2. Network & Host Topology Model (Context & Routing)

### 2.1 Split-Horizon / Multi-Hop CNAME Story

Public services hosted on an internal/home server (`edge-b`) can be exposed through a public VPS ingress node (`edge-a`) over NetBird while preserving topological visibility in DNS:

* `edge-a.example.com` $\to$ `A` $\to$ `203.0.113.10` (VPS Public IP)
* `edge-b.example.com` $\to$ `A` $\to$ `198.51.100.20` (Home Public IP, updated via Pulse)
* `via.edge-a.example.com` $\to$ `CNAME` $\to$ `edge-a.example.com`
* `via.edge-b.example.com` $\to$ `CNAME` $\to$ `edge-b.example.com`
* `on.edge-b.example.com` $\to$ `CNAME` $\to$ `via.edge-a.example.com` *(Designates that services on `edge-b` ingress through `edge-a`)*
* `jellyfin.example.com` $\to$ `CNAME` $\to$ `on.edge-b.example.com`

### 2.2 Host Roles & `external_nat` Handling

* **Managed Firewall Host (e.g., VPS `edge-a`):** Runs `nftables` managed by Hashi and a Traefik instance. Enforces L3/L4 folded IP sets, DNATs traffic to Traefik entrypoints, and runs L4 kernel meters.
* **Unexposed / NATed Host (`external_nat`, e.g., Home Proxmox Host `edge-b`):**
* Registered as a host in Hashi so Hashi tracks its public IP (via Pulse or DNS resolution) and executes SSH scripts on it.
* **No `nftables` artifacts are generated on `external_nat` hosts.**
* Its public IP is automatically included in the `Manual Allow IP Set` (`hashi_trusted`) of all Managed Firewall Hosts so home-to-VPS traffic is never blocked.
* Independent guest VMs/containers on the home network (e.g., local Traefik instance on a separate LAN IP, AdGuard Home DNS instance) connect independently to Hashi over NetBird.



---

## 3. Data Model: Recursive Primitives & Schemas

### 3.1 The Targeting Primitive: `IP Item` & `IP Set`

```
                        ┌──────────────────────┐
                        │        IP Set        │◄──────┐
                        │  - id, name          │       │ (Recursive Nesting
                        │  - items[]           │───────┘  with DAG Cycle Check)
                        └──────────┬───────────┘
                                   │ contains 1..N
          ┌────────────────────────┼────────────────────────┐
          ▼                        ▼                        ▼
┌───────────────────┐    ┌───────────────────┐    ┌───────────────────┐
│  Manual IP / CIDR │    │ Pulse Agent Ref   │    │ Geo / ASN Object  │
│  - 192.0.2.1/32   │    │ - agent_id        │    │ - Country (ISO)   │
│  - 10.0.0.0/24    │    │ - selector:       │    │ - Region          │
│  - 2001:db8::/32  │    │   internal |      │    │ - ASN (e.g. AS13) │
│                   │    │   public | all    │    │                   │
└───────────────────┘    └───────────────────┘    └───────────────────┘

```

#### `IP Item` Variants (Leaf Nodes)

1. **`ManualIP`:** Single IPv4 (`/32`) or IPv6 (`/128`) address.
2. **`ManualCIDR`:** Explicit IPv4 or IPv6 subnet block.
3. **`PulseAgentRef`:** Dynamic reference to a registered Pulse agent, paired with an address scope enum:
* `public`: Resolves to the agent's current reported public WAN IP(s).
* `internal`: Resolves to the agent's reported private/LAN/NetBird interface IP(s).
* `all`: Resolves to the union of both `public` and `internal` IPs.
* *Reactivity:* Whenever a Pulse agent heartbeats an IP change, Hashi triggers a reactive re-evaluation of all consuming `IP Sets`.


4. **`GeoASN`:**
* `Country` (e.g., `CH`, `DE`), `Region`, or `ASN` (e.g., `AS3303`).
* *Dual Compilation:* Compiled into underlying CIDR prefix lists (via MaxMind/IPinfo ASN & GeoLite2 CIDR blocks) when consumed by Layer 3/4 `nftables` sets, and evaluated via fast in-memory GeoIP lookup when consumed at Layer 7.



#### `IP Set` (Composite Container)

* Contains 1 or more child elements, where each element is either an `IP Item` or another `IP Set` ID.
* **Cycle Prevention Invariant:** On create/update of any `IP Set`, the backend runs a Depth-First Search (DFS) cycle detection check across the `IP Set` dependency graph. Any mutation introducing a cycle ($A \to \dots \to A$) is rejected with a `400 Bad Request`.

#### System Built-In `IP Sets`

1. **`All` (`sys_ipset_all`):** Immutable built-in set representing `0.0.0.0/0` and `::/0`.
2. **`External Blocklist IP Sets` (`ext_feed_*`):** Read-only sets populated by Hashi's background feed fetcher from configured URLs/threat lists. Retains last-known-good state if a remote feed fails to refresh.
3. **`Dynamic Challenge Required IP Set` (`sys_ipset_challenge`):** Runtime dynamic set holding IPs currently flagged by the Layer 7 Global Abuse Engine.
4. **`Dynamic Blocked IP Set` (`sys_ipset_blocked`):** Runtime dynamic set holding IPs currently banned across the entire platform (fed by failed L7 challenges and L4 kernel rate-limit breaches).

---

### 3.2 The Layer 7 Policy Primitive: `Rule Item`, `Rule Set`, & `Service`

```
                   ┌────────────────────────────────┐
                   │   Service (Root Rule Set)      │
                   │   - default_fallback: None|Deny│
                   └───────────────┬────────────────┘
                                   │ ordered by Priority (1, 2, 3...)
                   ┌───────────────┴────────────────┐
                   ▼                                ▼
         ┌───────────────────┐            ┌───────────────────┐
         │     Rule Item     │            │     Rule Set      │◄─────┐
         │ - priority: int   │            │ - priority: int   │      │ (Recursive
         │ - applies_to:     │            │ - entries[]       │──────┘  with DAG
         │     IP_Set_ID     │            └─────────┬─────────┘         Check)
         │ - condition:      │                      │ contains
         │     PathMatch?    │                      ▼
         │ - action:         │            (Rule Items & nested Rule Sets,
         │     None | Deny | │             each with its own local Priority)
         │     PassCAPTCHA | │
         │     PassAuth      │
         └───────────────────┘

```

#### `Rule Item` Schema (Leaf Node)

* **`priority` (`int`):** Strict ordering integer within its parent `Rule Set` (`1` = highest priority).
* **`applies_to` (`IP_Set_ID`):** Reference to any `IP Set` (including the built-in `All` set, custom sets, or Pulse-backed sets).
* **`condition` (`Optional<PathCondition>`):**
* `match_type`: `Exact` | `Prefix` | `Regex`
* `pattern`: String (e.g., `/admin`, `^/api/v[12]/private/.*`).
* If omitted/null, defaults to `Prefix: /` (matches all paths on the service).


* **`action` (`Enum`):**
1. **`None` (Allow Response):** Immediately grants access through Stage B. Skips any lower-priority custom `Pass CAPTCHA` or `Pass Authentication` rules.
2. **`Deny` (No Response):** Rejects the request at the reverse proxy (`403 Forbidden` or connection drop). Does **not** add the IP to any global dynamic set.
3. **`Pass CAPTCHA`:** Requires a valid service-scoped CAPTCHA clearance cookie/session before serving the response. Does **not** add the IP to `Dynamic Challenge Required IP Set`.
4. **`Pass Authentication`:** Requires a valid PocketID / OIDC SSO session before serving the response.



#### `Rule Set` Schema (Composite Node)

* A reusable named container holding an ordered list of entries, where each entry is either a `Rule Item` or a child `Rule Set` reference.
* Every entry inside a `Rule Set` has a mandatory unique `priority` integer within that container.
* **Cycle Prevention Invariant:** Saving a `Rule Set` runs a DAG cycle check across all nested `Rule Set` references.

#### `Service` as a Root `Rule Set`

* Every Layer 7 Service owns a root `Rule Set`. Users can insert standalone `Rule Items` directly onto the service and attach reusable `Rule Sets` at specific priority slots.
* **Deterministic Tree Flattening Algorithm:**
To evaluate rules without collisions between multiple attached `Rule Sets`, Hashi flattens the Service's `Rule Set` tree using **Pre-Order Depth-First Traversal** sorted by `priority` at each level, followed by the `Global Layer 7 Custom Rule Set`, and terminating at the Service's **Default Fallback Rule**:

$$\text{Default Fallback Rule: } \quad \text{Applies To: } \texttt{All}, \quad \text{Condition: } \texttt{Prefix "/"}, \quad \text{Action: } \texttt{None (or Deny)}$$


* *Why `Action: None` + Default Fallback is complete:*
* Standard open service with protected `/admin`: Keep Default Fallback = `None`, put `/admin` $\to$ `Pass Authentication` at Priority 1.
* Whitelist-only service where *only* `/public` is accessible: Set Default Fallback = `Deny`, put `/public` $\to$ `None` at Priority 1.
* Exemption ("Inverse") rule where Switzerland skips CAPTCHA and everyone else must solve CAPTCHA: Priority 1: `Applies To: Switzerland_IP_Set` $\to$ `Action: None`; Priority 2: `Applies To: All` $\to$ `Action: Pass CAPTCHA`.





---

## 4. Layer 3 / Layer 4 Firewall & Stream Engine

### 4.1 Firewall IP Set Folding & Longest-Prefix-Match Algorithm

On every Managed Firewall Host, Hashi compiles three logical IP set categories into a **single folded enforcement model**:

1. **`Combined Manual Allow Set` ($S_{\text{allow}}$):** The union of all `IP Sets` assigned to Global Firewall Allow + the auto-detected public IPs of all registered Hashi hosts (including `external_nat` home servers).
2. **`Combined Manual Block Set` ($S_{\text{block}}$):** The union of all `IP Sets` assigned to Global Firewall Block + all enabled `External Blocklist IP Sets`.
3. **`Dynamic Blocked IP Set` ($S_{\text{dyn\_block}}$):** The runtime set of IPs banned for duration $T_{\text{ban}}(c)$ by L7 failed challenges or L4 rate-limit breaches.

#### Precedence & Folding Rules

1. **Manual Allow vs. Manual Block (Longest Prefix Match):**
* Both $S_{\text{allow}}$ and $S_{\text{block}}$ are expanded into normalized CIDR intervals ($[ \text{start\_ip}, \text{end\_ip} ]$ with prefix length $p \in [0, 32]$ for IPv4 or $[0, 128]$ for IPv6).
* For any overlapping address range between an Allow CIDR (prefix $p_a$) and a Block CIDR (prefix $p_b$):
* If $p_a > p_b$ (Allow is more specific, e.g., a `/32` Pulse agent inside a blocked `/16` ASN or Country): the `/32` sub-range is carved out as **Allowed**.
* If $p_b > p_a$ (Block is more specific, e.g., a `/32` bad actor inside an allowed `/16` Country): the `/32` sub-range is carved out as **Blocked**.
* If $p_a = p_b$ (Exact tie on identical CIDR prefix): **Block wins** (fail-closed security default).




2. **Conscious Manual Decisions vs. Dynamic Bans:**
* Explicit manual decisions outrank dynamic runtime bans. Any CIDR/IP resolved as **Allowed** in Step 1 is subtracted/excluded from $S_{\text{dyn\_block}}$ before kernel injection.


3. **Final Folded `nftables` Representation:**
* Hashi writes the resulting effective blocked intervals (Manual Blocks minus Manual Allow punch-outs, unioned with Dynamic Blocks minus Manual Allow punch-outs) into `@hashi_folded_block` (an `nftables` interval set with `auto-merge`).



---

### 4.2 Layer 4 Proxy Philosophy & Kernel Metering (`TCP` / `UDP` / `QUIC`)

* **Routing Architecture:** Raw TCP, raw UDP, and non-HTTP QUIC streams are exposed by configuring `nftables` DNAT to Traefik's listening entrypoint ports, which then proxy the stream to the backend (directly or over NetBird). HTTP/3 uses Traefik's native UDP `:443` QUIC listener.
* **Access Scope Philosophy:** Published Layer 4 ports are public by design (subject to the global folded firewall blocklist). Services requiring private/restricted port access are not published on public L4 entrypoints and are accessed directly over NetBird.
* **Why Bandwidth is Excluded:** High-throughput legitimate workloads (multi-stream downloads, Nextcloud syncs, media streaming) legitimately saturate bandwidth, whereas low-bandwidth botnets evade byte-rate limits. Therefore, L4 abuse detection relies strictly on **connection establishment rates, concurrency, and packet rates (PPS)**.

#### The 4 Kernel Meters & Dynamic Staging Flow

```
[ Incoming L4 Packet ]
         │
         ▼
┌─────────────────────────────────────────────────────────────┐
│ nftables chain prerouting (priority raw)                    │
│ 1. ip saddr @hashi_folded_block -> DROP                     │
│ 2. ip saddr @hashi_l4_staging   -> DROP                     │
│ 3. Check 4 Kernel Meters:                                   │
│    • TCP New Conn Rate:  ct state new > R_tcp / window      │
│    • TCP Concurrency:    ct count > C_tcp_max               │
│    • UDP New Flow Rate:  ct state new > R_udp_flow / window │
│    • UDP Packet Flood:   rate > R_udp_pps / second          │
└────────┬────────────────────────────────────────────────────┘
         │ (If ANY meter breached)
         ▼
┌─────────────────────────────────────────────────────────────┐
│ Inline Kernel Action (0ms latency):                         │
│ add @hashi_l4_staging { ip saddr } drop                     │
└────────┬────────────────────────────────────────────────────┘
         │ (Polled every 2-5s by Hashi Host Agent via `nft -j`)
         ▼
┌─────────────────────────────────────────────────────────────┐
│ Hashi Central Security Engine (Hard Escalation):            │
│ 1. Parse JSON from `nft -j list set ... hashi_l4_staging`   │
│ 2. Skip if IP is covered by Manual Allow IP Set             │
│ 3. Increment Ban Counter: c(IP) = c(IP) + 1                 │
│ 4. Compute Ban Duration: T = T_ban(c(IP))                   │
│ 5. Add IP to global `Dynamic Blocked IP Set` for duration T │
│ 6. Atomic nft update on host:                               │
│    Add IP to @hashi_folded_block (timeout T) &              │
│    Delete IP from @hashi_l4_staging                         │
└─────────────────────────────────────────────────────────────┘

```

---

## 5. Layer 7 Security Engine (Stage A & Stage B)

Every HTTP/1.1, HTTP/2, and HTTP/3 request reaching Traefik passes through two sequential stages via Hashi's security middleware / forward-auth engine.

### 5.1 Stage A: Global Immutable Abuse Engine (`Applies To: All`)

Stage A runs before any custom service or group rules. It protects all endpoints globally, **including SSO login pages and CAPTCHA verification endpoints**.

#### Step A1: Global Block Sync Guard

* Checks if `client_ip` is in `Dynamic Blocked IP Set` (or `Combined Manual Block Set` minus `Manual Allow`).
* **If matched:** Immediately terminate request (`403 Forbidden` / connection close). This prevents a fast attacker from slipping requests through Traefik during the 1–2 second window while `nftables` synchronizes across hosts.

#### Step A2: Per-Service / Per-Path WAF Inspection & Global Rate Limiting

* **WAF Mode Resolution:** Checks the target Service's WAF configuration for the request path (most specific path match wins, falling back to Service Root `/`, falling back to Global Default):
* **`Enabled` (Default):** Coraza runs with OWASP CRS (`SecRuleEngine On`, `SecRequestBodyAccess On`). Inspects URI, headers, cookies, and POST/PUT bodies.
* **`Headers Only`:** Coraza runs with `SecRuleEngine On`, `SecRequestBodyAccess Off`. Inspects URI, query params, cookies, and headers, but ignores request bodies (prevents false positives on Nextcloud WebDAV, Git pushes, file uploads, and rich JSON APIs).
* **`Disabled`:** Coraza bypassed (`SecRuleEngine Off`) for matching path/service.


* **Immediate Attack Rejection:** If an active WAF rule triggers a blocking match (e.g., SQLi in query string), the malicious request itself is rejected immediately, and the IP's short-term WAF violation tally is incremented.
* **Threshold Check (WAF & HTTP Request Rate):**
* If the IP's WAF violations exceed `Global WAF Violation Threshold` ($N_{\text{waf}}$ within $\Delta t_{\text{waf}}$) **OR** total HTTP requests exceed `Global L7 Rate Limit Threshold` ($R_{\text{http}}$ within $\Delta t_{\text{http}}$):
* The IP is added to the **`Dynamic Challenge Required IP Set`**.
* The IP's challenge entry expiration timestamp is set to `now + T_exp` (where $T_{\text{exp}}$ is a fixed global setting, **not** a counter function).
* The IP's active challenge failure counter $f(\text{IP})$ is initialized to `0`.





#### Step A3: Global Challenge Enforcement (`Applies To: Dynamic Challenge Required IP Set`)

* If `client_ip` is currently in `Dynamic Challenge Required IP Set`:
* **Precedence over SSO:** This check runs *before* authentication. If someone floods the SSO login page, Stage A intercepts them here with a CAPTCHA before they can touch PocketID/SSO.
* **Case 1 — Valid CAPTCHA Solution Submitted:**
* Remove `client_ip` from `Dynamic Challenge Required IP Set`.
* Reset active failure counter $f(\text{IP}) \leftarrow 0$.
* Allow request to proceed to Stage B.


* **Case 2 — Fixed Timer $T_{\text{exp}}$ Elapsed:**
* Lazy/background expiration removes `client_ip` from `Dynamic Challenge Required IP Set` once `now >= expires_at`.


* **Case 3 — Request Made Without Solving CAPTCHA (Failure):**
* Every request hitting the server from this IP that is not a successful CAPTCHA verification increments the active failure count: $f(\text{IP}) \leftarrow f(\text{IP}) + 1$.
* **If $f(\text{IP}) < F_{\max}$ (`Max Failed Challenge Attempts`):** Serve the CAPTCHA challenge page (`429` / `403` challenge response).
* **If $f(\text{IP}) \ge F_{\max}$ (Escalation to Global Block):**
1. Remove `client_ip` from `Dynamic Challenge Required IP Set`.
2. Increment the IP's persistent **Ban Counter**: $c(\text{IP}) \leftarrow c(\text{IP}) + 1$.
3. Calculate ban duration using the configured function: $T_{\text{ban}}(c(\text{IP}))$.
4. Add `client_ip` to the **`Dynamic Blocked IP Set`** with expiration `now + T_ban(c)`.
5. Push update to all Managed Firewall Hosts (`@hashi_folded_block` in `nftables`) and drop the request.







---

### 5.2 Stage B: Custom Access Rule Tree Evaluation

Requests that pass Stage A cleanly are evaluated against the Service's flattened `Rule Set` list:

```
Flatten(Service.RootRuleSet) + Flatten(GlobalCustomRuleSet) + [Service.DefaultFallbackRule]

```

For each `Rule Item` in priority order (`1..N`):

1. **Check `Applies To` (`IP Set`):** Does `client_ip` belong to the rule's target `IP Set` (evaluating nested sets, CIDRs, Pulse agent IPs, and GeoIP/ASN)? If no $\to$ continue to next rule.
2. **Check `Condition` (`Path`):** Does `request.path` match the `Exact`, `Prefix`, or `Regex` pattern (or `/` if omitted)? If no $\to$ continue to next rule.
3. **Execute First Matching `Action` and Terminate Traversal:**
* **`Action: None`:** Request is permitted; forward to backend service.
* **`Action: Deny`:** Return `403 Forbidden` (or close connection). Do **not** modify global IP sets.
* **`Action: Pass CAPTCHA`:**
* Check if the user has a valid signed CAPTCHA clearance session/cookie for this service.
* If valid $\to$ forward to backend service.
* If missing $\to$ present the CAPTCHA challenge for this service. *(Note: The IP is **not** added to `Dynamic Challenge Required IP Set`. However, if a bot hammers this page and ignores the CAPTCHA, its requests are still counted by Step A2's `Global L7 Rate Limit Threshold`, which will automatically move the IP into Stage A's `Dynamic Challenge Required IP Set` and subsequently ban it on the firewall).*


* **`Action: Pass Authentication`:**
* Check if the user has a valid SSO session token/cookie.
* If valid $\to$ forward to backend service.
* If missing $\to$ redirect/prompt for SSO login. *(Just like `Pass CAPTCHA`, brute-forcing or flooding the authentication flow trips Step A2's global rate limiter/WAF, escalating the IP into Stage A's challenge and firewall ban pipeline).*





---

## 6. Exhaustive Global Settings & Functions Reference

All configurable settings exist at the global engine level (with per-service overrides strictly limited to the Service Rule Set, Default Fallback toggle, and Per-Service/Path WAF Mode).

### 6.1 Numerical & Mode Settings

| Setting Key | Type | Default Recommendation | Description |
| --- | --- | --- | --- |
| `waf.default_mode` | Enum (`Enabled`, `HeadersOnly`, `Disabled`) | `Enabled` | Global baseline Coraza + OWASP CRS mode for services that don't specify an override. |
| `waf.violation_threshold` | Int ($N_{\text{waf}}$) + Window ($\Delta t_{\text{waf}}$) | `5 hits / 60s` | Number of WAF rule matches from a single IP within $\Delta t_{\text{waf}}$ before moving the IP into `Dynamic Challenge Required IP Set`. |
| `l7.rate_limit_threshold` | Int ($R_{\text{http}}$) + Window ($\Delta t_{\text{http}}$) | `300 req / 10s` | Max Layer 7 HTTP requests from a single IP within $\Delta t_{\text{http}}$ before moving the IP into `Dynamic Challenge Required IP Set`. |
| `challenge.expiry_time` | Fixed Duration ($T_{\text{exp}}$) | `30m` (or `0` = infinite) | Fixed duration an IP stays in `Dynamic Challenge Required IP Set` before automatic clearance. Independent of any counter. |
| `challenge.max_failed_attempts` | Int ($F_{\max}$) | `15 requests` | Number of unsolved requests allowed while in `Dynamic Challenge Required IP Set` before escalating to `Dynamic Blocked IP Set`. |
| `l4.tcp_new_conn_rate` | Int ($R_{\text{tcp}}$) + Burst ($B_{\text{tcp}}$) / Window | `150/min` (burst `30`) | `nftables` meter threshold for new TCP SYN connections (`ct state new`) per IP before L4 Hard Escalation. |
| `l4.tcp_max_concurrent` | Int ($C_{\text{tcp\_max}}$) | `250 connections` | `nftables` threshold for simultaneous active TCP connections (`ct count`) per IP before L4 Hard Escalation. |
| `l4.udp_new_flow_rate` | Int ($R_{\text{udp\_flow}}$) + Burst / Window | `200/min` (burst `50`) | `nftables` meter threshold for new UDP conntrack flows (`ct state new`) per IP before L4 Hard Escalation. |
| `l4.udp_pps_rate` | Int ($R_{\text{udp\_pps}}$) + Burst / Second | `5000/sec` (burst `1000`) | `nftables` meter threshold for raw UDP packets per second per IP before L4 Hard Escalation. |
| `ban.counter_decay_ttl` | Duration ($T_{\text{decay}}$) | `7 days` | Time after an IP's last ban expires with zero new bans before its Ban Counter $c(\text{IP})$ resets to `0`. |
| `feeds.refresh_interval` | Duration | `6 hours` | Polling interval for external IP blocklist feeds. |
| `feeds.stale_expiry_ttl` | Duration | `72 hours` | Maximum time to retain last-known-good feed entries if a remote blocklist URL is unreachable. |

### 6.2 The Ban Duration Function $T_{\text{ban}}(c)$

Whenever an IP enters the `Dynamic Blocked IP Set` (either by reaching $F_{\max}$ failed challenges in L7 Stage A, or by triggering an L4 kernel meter), its Ban Counter $c \in \{1, 2, 3, \dots\}$ is incremented first ($c \leftarrow c + 1$), and its ban duration is computed according to the selected global formula:

* **Parameters:**
* `function_type`: `Constant` | `Linear` | `Exponential`
* $T_{\text{base}}$: Base ban duration (e.g., `15m`)
* $k$: Multiplier / slope factor (e.g., `2.0`)
* $T_{\max}$: Maximum cap duration (e.g., `30d`)
* $C_{\text{perm}}$: Optional permanent ban threshold (e.g., `0` = disabled, or `10` = permanent ban on 10th offense)


* **Evaluation Formula:**

$$T_{\text{ban}}(c) = \begin{cases} \infty \text{ (Permanent)} & \text{if } C_{\text{perm}} > 0 \text{ and } c \ge C_{\text{perm}} \\ \min\Big(T_{\max},\; T_{\text{base}}\Big) & \text{if type} = \texttt{Constant} \\ \min\Big(T_{\max},\; T_{\text{base}} \times \big(1 + k \cdot (c - 1)\big)\Big) & \text{if type} = \texttt{Linear} \\ \min\Big(T_{\max},\; T_{\text{base}} \times k^{(c - 1)}\Big) & \text{if type} = \texttt{Exponential} \end{cases}$$



---

## 7. Special Edge Cases & How the Architecture Solves Them

| Edge Case / Scenario | How the Specification Handles It |
| --- | --- |
| **1. You accidentally trigger an L4 or L7 rate limit from your own home IP or laptop.** | 1. Your home server (`edge-b`) public IP and your Pulse-monitored devices (`Trusted_Devices` IP Set) can be placed in the Global Firewall Allow Set. Because **Manual Decisions override Dynamic Bans**, even if an L4 meter catches your IP in `@hashi_l4_staging`, the folding logic refuses to block a manually allowed `/32`.<br>

<br>2. Even if an IP isn't in Global Firewall Allow, the UI's **IP Sets tab** directly exposes `Dynamic Challenge Required IP Set` and `Dynamic Blocked IP Set` with an event log and a 1-click "Remove / Reset Counter" action. |
| **2. Nextcloud / Gitea / Jellyfin breaks when uploading large files or syncing WebDAV.** | Set the Service's WAF mode on `/remote.php/dav` (or the whole service root `/`) to **`Headers Only`**. Coraza still protects URI and headers against exploits, but skips `SecRequestBodyAccess`, eliminating payload false positives and saving CPU/RAM on large uploads. |
| **3. An attacker floods the PocketID / SSO login page on a `/admin` route.** | Even though `/admin` is gated by a Stage B `Pass Authentication` rule, **Stage A runs first**. Once the attacker's login requests cross `l7.rate_limit_threshold` or `waf.violation_threshold`, Stage A puts the IP into `Dynamic Challenge Required IP Set`. Because Stage A takes precedence over Stage B, the attacker is locked behind a CAPTCHA before touching SSO, and after $F_{\max}$ ignored requests, gets banned on the `nftables` firewall. |
| **4. You want a service where ONLY `/api/public` is accessible to the world, and everything else is dead — except for your devices.** | Set the Service's **Default Fallback Rule** to `Action: Deny`. Add Priority 1: `Applies To: Trusted_Devices_IP_Set`, `Condition: /`, `Action: None`. Add Priority 2: `Applies To: All`, `Condition: Prefix /api/public`, `Action: None`. Any other path hits the bottom fallback and gets denied. |
| **5. You attach 3 different reusable `Rule Sets` to a Service, and two of them have conflicting rules for `/admin`.** | Because every entry attached to the Service's Root `Rule Set` has a mandatory integer `Priority` (`1, 2, 3...`), the `Rule Set` assigned Priority 1 is traversed completely before Priority 2. There is never an ambiguous tie. |
| **6. Your home connection's dynamic WAN IP changes at 3 AM.** | The Pulse agent on your home host reports the new public IP to Hashi (or Hashi resolves the updated DNS record). Hashi automatically updates every `IP Set` containing that Pulse Agent Reference, recomputes the folded `nftables` set (`@hashi_folded_block` and trusted host allowances), and pushes the atomic diff to your VPS firewalls. |
