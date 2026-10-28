# 06 — Edge security: two-stage engine, rule sets, SSO, CAPTCHA, WAF, feeds

Every HTTP-family request to a service passes Hashi's forward auth: **Stage A**, the global abuse engine, then
**Stage B**, the service's rule set. L4 services are protected at the firewall (kernel meters, folded sets,
[05-firewall.md](05-firewall.md)). Addresses always come from IP sets ([05a-ip-sets.md](05a-ip-sets.md)). Tables:
[06a-security-data-model.md](06a-security-data-model.md). Decision record: [ADR 0002](../adr/0002-security-model.md).

**Changed from the first spec:** manual entries with scopes and bypass flags, security subjects and their eight states,
soft blocks, blocklist enforcement modes, per-resource policy modes (Off / SSO required / Adaptive / Observe), WAF
detect mode and resource rules are gone. They are replaced by IP sets, rule sets, two dynamic IP sets and one ban
function.

## 1. Principles

1. **Rules vs settings.** Rules are deterministic mappings (who · what condition → action), edited per service and in
   reusable rule sets. Settings are global numbers, timers and the ban function. Per service, only the rule set, the
   default fallback and the WAF mode per path are configurable.
2. **Two recursive primitives.** *Who* is an IP set (05a). *L7 access policy* is a rule set; a service owns a root rule set.
3. **No ambiguity.** Firewall: longest prefix match, tie → block, manual beats dynamic (05 §4). L7: Stage A before
   Stage B; inside Stage B one flattened list, first match wins.
4. **Scoped vs global state.** Service rules (`Pass CAPTCHA`, `Pass Authentication`, `Deny`) only gate that service and
   never touch the dynamic sets. Abuse (WAF hits, request floods, ignored challenges, L4 floods) is tracked globally and
   escalates to a ban on every firewall host.

## 2. Forward auth

`GET /api/edge-auth/forward` (Traefik `forwardAuth`, `trustForwardHeader: true` only because Traefik hosts are trusted proxies).

Input from Traefik: `X-Forwarded-Host`, `X-Forwarded-Uri`, `X-Forwarded-Method`, `X-Forwarded-Proto`, `X-Forwarded-For`,
plus `Cookie`, `Accept`, `User-Agent`. The client address comes from the client-IP resolver (01-platform.md §8).

Responses: `204` allow (with `X-Hashi-User` when SSO-authenticated, configurable auth response headers); `302` to login
or challenge for browsers; `401` authentication required (API clients); `403` denied or banned; `429` challenge required
for API clients (`{"error":"challenge_required","challengeUrl":…}`).

Service resolution uses host **and** path routing exactly as Traefik does (most specific route wins). The request path
is normalized once (percent-decoding of unreserved characters, dot-segment removal, duplicate slashes collapsed) and every
path condition and WAF path mode is matched against the normalized path.

Forward auth is attached to **every** HTTP-family service, system services included (admin, challenge). There is no "off"
mode: Stage A protects everything.

### 2.1 When Hashi is unreachable

Traefik cannot "fail open" natively, so every Traefik host gets a tiny **local fallback**: forward auth targets a local
route whose service has a health check on Hashi and a failover to a static `204` responder.

| Service | During a Hashi outage |
| --- | --- |
| Flattened rules contain a `Deny` or `Pass Authentication` item, or the fallback is `Deny` | Fail **closed**: points straight at Hashi, no fallback |
| Only `None` / `Pass CAPTCHA` | Fail **open** through the local fallback: Stage A and CAPTCHA pause |
| Firewall sets (`block`, `dyn`, `staging`) and meters | Keep working in the kernel |
| Recording | The outage window is recorded as an event; access logs are ingested afterwards |

Verify the fallback construct against the pinned Traefik version and prove the outage path with a real Traefik in S4.
Forward auth for services on a remote Traefik host crosses the overlay network; if that link drops, the same table applies.
S9 records the forward-auth latency over that link.

### 2.2 Performance budget

p99 ≤ 5 ms in-process. All inputs live in memory: compiled IP sets (interval lists), flattened rule lists per service,
dynamic set state, per-address counters, GeoIP reader. Refreshed on change notifications and every 30 s. Event recording
is async through a bounded channel; on overflow, events are sampled and a counter records drops — the decision is never
delayed by logging.

## 3. Stage A — global abuse engine

Runs first for every request on every service, including SSO login flows and the CAPTCHA challenge service.

### 3.1 A1 — block guard

If the client address is in the folded `block` ranges or in `dynamic-blocked` (and not in the folded `allow` ranges —
05 §4.2, the same compiled partition the firewall uses): `403`, stop. This closes the seconds-long window before a new
ban reaches every firewall host.

### 3.2 A2 — WAF and request rate

**WAF** (§9) runs in Traefik before the request reaches the upstream. A blocking match rejects that request at the proxy.
Hashi learns of every match from the continuously tailed access log (§6.2) and counts it for the address.

**Rate:** forward auth counts every request per address in a sliding window.

**Trigger:** when an address exceeds `waf.violation_threshold` (N matches within Δt) **or** `l7.rate_limit_threshold`
(R requests within Δt), Hashi adds it to `dynamic-challenge` with `expires_at = now + T_exp` and failure count `f = 0`, and increments the
address's challenge counter (§5.1).

**Exempt from the trigger:**
- addresses in the folded `allow` ranges — never added to any dynamic set (invariant 20);
- requests carrying a valid edge SSO session — not counted towards the rate trigger. Their WAF matches still count.

IPv6 addresses are tracked, challenged and banned per /64.

### 3.3 A3 — challenge enforcement

If the address is in `dynamic-challenge`:

| Case | Effect |
| --- | --- |
| Valid CAPTCHA solution submitted (`POST /api/edge-challenge/verify`, scope `global`) | Remove from `dynamic-challenge`, `f ← 0`; the request continues to Stage B. The ban counter is **not** reset |
| `now ≥ expires_at` | Entry removed (lazy check + cleanup job). `T_exp = 0` means no expiry |
| Any other request | `f ← f + 1`. If `f < F_max`: browser → `302` to the challenge page; API → `429 challenge_required`. If `f ≥ F_max`: escalate (§3.4), `403` |

Requests to the challenge flow itself (challenge page, its assets, verify) never count as failures and stay reachable.
A3 runs before authentication: someone flooding an SSO login is stopped by a CAPTCHA before reaching the IdP.

Browser vs API: `Accept` contains `text/html` and method is GET/HEAD → browser redirect to
`/api/edge-challenge/start?scope=global&rd=…` preserving the original URL (unsafe/cross-origin `rd` → service root).

### 3.4 Escalation to a ban

Sources: A3 reaching `F_max`; an L4 meter (address drained from a host's staging set, 05 §6); "ban now" by the admin.

1. Skip if the address is in the folded `allow` ranges (record an event; no ban).
2. Remove it from `dynamic-challenge` if present.
3. Ban counter `c ← c + 1`.
4. `T = T_ban(c)` (§5.2).
5. Add to `dynamic-blocked` with `expires_at = now + T` (none when permanent), source and reason.
6. The host link pushes it to `dyn` on every `managed` host (05 §6); A1 enforces it immediately.

### 3.5 Dynamic sets

`dynamic-challenge` and `dynamic-blocked` are IP sets of kind `dynamic` (05a §3): visible on the IP sets page with every
entry, its source, reason, expiry, failure count and the address's counters; manual actions remove, add with a duration,
edit counters (§5.1). The Counters view (§13.1) lists every address with counters, active or not.
Both dynamic sets may be used in rule `Applies To` like any set.

## 4. Stage B — rule sets

### 4.1 Rule item

| Field | Values |
| --- | --- |
| `priority` | integer, unique within its container, `1` first |
| `applies_to` | an IP set (any kind: `all`, user, feed, `hashi-hosts`, Pulse-backed …) |
| `condition` | optional path match: `exact` \| `prefix` \| `regex` (RE2, compiled at save) + pattern starting with `/`; omitted = prefix `/` |
| `action` | `none` \| `deny` \| `pass_captcha` \| `pass_authentication` |
| `auth` | for `pass_authentication` only: optional provider (default provider when empty), optional allowed groups |
| `enabled`, `note` | |

Actions:

- **`none`** — allow; Stage B ends; the request goes to the upstream. Skips every lower-priority `pass_captcha` and
  `pass_authentication`.
- **`deny`** — `403`. Never touches dynamic sets.
- **`pass_captcha`** — requires a valid clearance cookie for this service: HMAC-signed `{service, issued, expiry}`,
  lifetime `captcha.clearance_ttl`, not bound to the address (mobile clients change addresses). Missing → challenge with
  `scope=service`. Never adds the address to `dynamic-challenge`; a bot hammering the challenge is still counted by A2.
  A valid edge SSO session (§7) also satisfies it, so a logged-in user is never asked for a CAPTCHA (e.g. on a VPN that a
  rule sends to CAPTCHA).
- **`pass_authentication`** — requires a valid edge SSO session (§7); with allowed groups, a session outside them gets
  `403`. Missing → login redirect (browser) or `401` (API). Floods of the login flow are counted by A2.

### 4.2 Rule set and service root

A rule set is a named, reusable container of entries; each entry is a rule item or a reference to another rule set, with
a priority unique in that container. Rule set references are acyclic (DFS on save, `400` with the path). A rule set
attached anywhere cannot be deleted (the error lists where).

Global, service-root and reusable rule sets are **one type** (`kind`: `user` | `global` | `service_root`, plus `origin`,
06a §0). Every HTTP-family service owns a **root rule set** (not attachable elsewhere, deleted with the service) whose last
entry is the **default fallback**: a real rule item with origin `system`, locked to `applies_to all`, prefix `/` and the
last position; only its action is editable — `none` (open service) or `deny` (closed service). The one rule set of kind
**`global`** holds rules that apply to every service after its own rules; it cannot be deleted.

### 4.3 Flattening and evaluation

```
Flatten(service root without its fallback) + Flatten(global) + [the service root's fallback item]
```

Stage A appears at the top of every trace and flattened view as locked, read-only entries in the same shape (A1 block
guard, A2 trigger, A3 challenge) so the whole path reads as one list; it is implemented as engine code, not as rule items,
because A2 is a windowed counter rather than a match → action rule and Stage A must not be removable.

`Flatten` is a pre-order depth-first traversal sorted by priority at every level: a rule set at priority 1 is traversed
completely before the entry at priority 2. Disabled items are skipped. The flattened list is recomputed on every change
of a rule set, an attachment or the global set, and cached per service.

For each item in order: the address is in `applies_to` (05a §4.1, including geo predicates) **and** the path matches the
condition → execute the action and stop. Every decision returns the trace: Stage A steps, the flattened list with the
deciding item and the rule-set path it came from (`service root → shared/admin-protect → item 2`).

Save-time warnings (not errors): an item after an earlier `applies_to all` + prefix `/` item is unreachable;
`none` for `all` at priority 1 disables all later protection on that service; a `geo` set used while GeoIP is missing.

### 4.4 Patterns

| Goal | Rules |
| --- | --- |
| Open service with protected `/admin` | fallback `none`; 1: `all` · prefix `/admin` → `pass_authentication` |
| Only `/public` reachable | fallback `deny`; 1: `all` · prefix `/public` → `none` |
| Switzerland skips CAPTCHA, everyone else solves it | 1: `switzerland` → `none`; 2: `all` → `pass_captcha` |
| Only `/api/public` for the world, everything for my devices | fallback `deny`; 1: `trusted-devices` → `none`; 2: `all` · prefix `/api/public` → `none` |
| Conflicting `/admin` rules in two attached rule sets | the set at the lower priority number wins, completely traversed first |

### 4.5 System services

| Service | Stage A | Stage B |
| --- | --- | --- |
| Hashi admin | A1–A3 | fallback `none` (admin auth is Hashi's own passkey session) |
| CAPTCHA challenge (when Cap is enabled) | A1 only (challenged addresses must reach it; its requests never count as failures) | none |

## 5. Ban function and counters

### 5.1 Counters

Each address (IPv6: /64) has one persistent counter record with two counters:

| Counter | Increments | Quiet time starts | Effect |
| --- | --- | --- | --- |
| **Ban counter** `c` | before every ban (§3.4), any source | at the end of the last ban (a permanent ban never decays) | drives `T_ban(c)` |
| **Challenge counter** | each time the address enters `dynamic-challenge` (§3.2, or "challenge now") | at the last challenge | informational (stats, sorting, filters); `T_exp` stays fixed |

**Step decay:** each counter drops by **1** for every full `ban.counter_decay_ttl` (`D`) of quiet time, down to 0. An
address banned once is forgotten after `D`; one banned six times needs `6 × D` without a new ban. A new ban (challenge)
restarts that counter's quiet time; a violation that does not lead to a ban leaves the ban counter and its clock alone.

Decay is computed at read time, never by a timer per address. The record stores the counter and its quiet-since time;
the effective value is

```
effective(n, since) = n                                  if now ≤ since
                    = max(0, n − ⌊(now − since) / D⌋)    otherwise
```

On an increment Hashi first takes the effective value, adds 1 and stores it with the new quiet-since time (for a ban: its
end). Every view, filter and `T_ban` use effective values; the Counters view also shows when the next decrement happens.
Neither counter is reset by a CAPTCHA solve. The live failure count `f` belongs to the current challenge entry (§3.3),
not to the counter record. A record whose effective counters are 0 and that has no active entries or events for 30 days
is deleted by the cleanup job.

Manual actions (reauth, audited): set either counter to a value (stored as the effective value; quiet time restarts at
the later of now and the end of a running ban; setting `c` changes the next ban's duration, not a running ban), reset one
or both, reset `f` of a running challenge. Bulk reset applies to a selection or to every row matching the current filter
(the confirmation names the count).

### 5.2 Ban duration

```
T_ban(c) = ∞                                        if C_perm > 0 and c ≥ C_perm
         = min(T_max, T_base)                       if type = constant
         = min(T_max, T_base × (1 + k·(c − 1)))     if type = linear
         = min(T_max, T_base × k^(c − 1))           if type = exponential
```

Parameters (settings): `ban.function` (`constant` | `linear` | `exponential`), `ban.base` (`T_base`, 15 m), `ban.k`
(2.0), `ban.max` (`T_max`, 30 d), `ban.permanent_after` (`C_perm`, 0 = never). The settings page plots `T_ban(1…12)`.

## 6. Telemetry

### 6.1 Events and buckets

Kept from the first spec, with the address as the subject:

- `security_buckets_1m`: bucket start, service, root domain, Traefik connection, country, ASN, decision/action, status
  class, path prefix (first segment, ≤ 50 per service per minute, else `other`), request/denied/challenged/
  challenge-failed/banned counts. **No address.**
- `security_ip_top_1m`: top 500 addresses per closed minute (address, service, counts).
- `security_events`: only non-allow decisions and state changes (challenge entered/solved/expired/escalated, ban
  created/applied per host/expired/removed, WAF match, meter trip, manual actions). At most 10 events per address per
  minute; further ones increment `suppressed` on the last stored event.

Per-address counters (rate window, WAF window, `f`) live in memory; `c` and dynamic entries are persisted.

### 6.2 Access-log tail

Hashi follows `/var/log/hashi/traefik/access.log` on every Traefik host over a persistent SSH session (cursor = inode +
byte offset; rotation handled by finishing the old inode from `.1`), parsing JSON lines as they arrive (target lag: a
few seconds). WAF matches come from the Coraza plugin's fields in the same stream. On reconnect it resumes from the
cursor, bounded per read (8 MB).

## 7. Edge SSO

- One or more OIDC providers (issuer discovery, client ID, client secret in vault, scopes, claim mapping for
  username/email/groups). One is the default.
- `pass_authentication` uses its item's provider, else the default; with a default provider Hashi redirects straight to it.
- Edge session cookie on the root domain so one login covers all subdomains; HMAC-signed session ID, server-side
  `edge_sessions` row; settings: max session length, idle timeout, remember-device.
- Flow: `/api/edge-auth/login?rd=<url>` → IdP (PKCE + state + nonce) → `/api/edge-auth/callback` → validate `rd` is a
  managed domain (no open redirect) → set cookie → redirect. `/api/edge-auth/logout` clears and optionally calls IdP
  end-session. These endpoints run Stage A in-process.
- A valid edge session exempts its requests from the A2 rate trigger only (§3.2); never from A1, A3, WAF or rules.

## 8. Feeds (external blocklists)

A feed is an IP set of kind `feed` (05a §3). What it does depends only on where it is used: bound to *Global firewall
block* → kernel + A1; used in a rule's `applies_to` → that rule only.

- Offered individually in setup and on the IP sets page; **all disabled until explicitly added**; warning about false positives.
- Recommended: Feodo Tracker botnet C2 IPs, Spamhaus DROP, Spamhaus DROPv6, DShield recommended block list, FireHOL
  Level 1. Plus custom URL.
- Each shows: name, URL, description, false-positive risk, refresh interval, format, licence/user-responsibility note,
  last success, entry count, where it is used.
- Formats: plain IP/CIDR lines, with comments, FireHOL `.ipset`/`.netset`, CSV/TSV (configurable column), JSON array of
  strings, JSON object with a configured array field. Out of scope: Suricata/Snort rules, domain/URL lists.
- Parsing: normalize IPv4/IPv6/CIDR, reject invalid, dedupe, keep line numbers, record errors and ignored lines, preview
  before adding, never partially apply a failed update.
- Fetcher (SSRF-safe): HTTPS only by default (HTTP only with explicit warning), TLS verification, max redirects, max size,
  timeout, Hashi user agent, ETag/Last-Modified. Validate the URL, every DNS answer, every redirect target **and the
  final dialed IP** (custom `net.Dialer.Control` hook) against denied ranges: loopback, RFC 1918, link-local, multicast,
  ULA, IPv6 link-local, cloud metadata ranges, Docker bridge ranges (unless dev mode).
- Refresh every `feeds.refresh_interval`. A failed refresh keeps the last-known-good entries and marks the set `partial`;
  after `feeds.stale_expiry_ttl` without success the set resolves empty and Overview shows the stale feed. Fetch runs are
  kept (counts parsed/added/removed/unchanged/rejected, content hash).

## 9. WAF

Coraza Traefik plugin with OWASP CRS, free, local, pinned version (verified to exist in the plugin catalog), full CRS.

| Mode | Coraza | Use |
| --- | --- | --- |
| `enabled` (default) | `SecRuleEngine On`, `SecRequestBodyAccess On` | URI, headers, cookies and bodies inspected |
| `headers_only` | `SecRuleEngine On`, `SecRequestBodyAccess Off` | WebDAV, git pushes, uploads, rich JSON APIs |
| `disabled` | no WAF middleware | |

Resolution per request: most specific path mode of the service → the service's root mode → `waf.default_mode`. Per-path
modes are rendered as additional Traefik routers with the path match and a higher priority, each with its own WAF
middleware instance. Per service/path rule exclusions (CRS rule IDs) handle false positives without disabling the WAF.
Hashi never implements a WAF engine.

## 10. Settings

All in Settings → Security (09-scripts-settings.md §2).

| Key | Default | Meaning |
| --- | --- | --- |
| `waf.default_mode` | `enabled` | baseline WAF mode |
| `waf.violation_threshold` | 5 within 60 s | WAF matches per address → challenge |
| `l7.rate_limit_threshold` | 300 within 10 s | requests per address → challenge (exempt: allowed addresses, SSO sessions) |
| `challenge.expiry` (`T_exp`) | 30 m (0 = never) | fixed lifetime of a challenge entry |
| `challenge.max_failed` (`F_max`) | 15 | unsolved requests while challenged → ban |
| `captcha.clearance_ttl` | 24 h | `pass_captcha` cookie lifetime |
| `l4.tcp_new_conn_rate` | 150/min, burst 30 | kernel meter |
| `l4.tcp_max_concurrent` | 250 | kernel meter (per source) |
| `l4.udp_new_flow_rate` | 200/min, burst 50 | kernel meter (L4 UDP stream ports) |
| `l4.udp_pps_rate` | 5000/s, burst 1000 | kernel meter (L4 UDP stream ports) |
| `l4.staging_timeout` | 5 m | kernel drop time before Hashi drains an entry |
| `l4.poll_interval` | 3 s | host link drain/push interval |
| `ban.function`, `ban.base`, `ban.k`, `ban.max`, `ban.permanent_after` | exponential, 15 m, 2.0, 30 d, 0 | §5.2 |
| `ban.counter_decay_ttl` | 7 d | quiet time per step of decay (−1) of the ban and challenge counters (§5.1) |
| `feeds.refresh_interval` | 6 h | feed refresh |
| `feeds.stale_expiry_ttl` | 72 h | last-known-good retention |
| `geoip.update` | weekly | MaxMind download |

Known risk, accepted: `F_max` counts every request while challenged, so apps that poll in the background can reach it
quickly; challenges are per address, so people behind one shared address (CGNAT) share a challenge. The defaults are a
starting point; the dashboard shows challenge → ban conversions so they can be tuned.

## 11. CAPTCHA (Cap)

Hashi integrates with an **existing** Cap Standalone instance; it does not deploy Cap.

Setup/settings: enable, Cap public challenge base URL, site key (visible), secret key (service secret), verification
timeout, public challenge service domain, optional Cap admin service domain (recommend `pass_authentication`), test
verification.

- **Challenge service** (system, required while CAPTCHA is enabled; edit only via CAPTCHA settings): §4.5; exposes only
  the paths the challenge flow needs; never the Cap admin dashboard or management API; a real reverse-proxy domain.
- **Cap admin service** (optional, normal, user-deletable): no `pass_captcha` (dependency loop).

One challenge page serves both scopes: `scope=global` (A3: a solve removes the address from `dynamic-challenge`) and
`scope=service` (`pass_captcha`: a solve sets the clearance cookie for that service). The page is minimal, loads only
required assets, shows the domain being accessed, submits the Cap token to `POST /api/edge-challenge/verify`, handles
failure without leaking details and never exposes the Cap secret. Hashi verifies the token with Cap server-side.

## 12. GeoIP

MaxMind GeoLite2 Country + ASN (account ID + licence key in vault; optional City for regions if the licence allows).
Download job with checksum, atomic swap of the reader, staleness warning after 14 days. Used for `geo` items at L7
(lookup) and at the firewall (prefix expansion, 05a §4.3), and for country/ASN columns in telemetry. Without databases
`geo` items resolve to nothing and widgets show "GeoIP unavailable".

## 13. Security UI

- **Security** (`/security`): Dashboard (allowed / challenged / banned / WAF tiles with sparklines; L4 meter trips per
  host; top addresses, countries, ASNs, targeted services; challenge → solve → ban funnel; recent events; stale feeds and
  GeoIP), Counters (§13.1), Challenge (Cap), Sign-in providers, Engine (§10 settings, `T_ban` curve).
  Filters: 1 h / 24 h / 7 d / 30 d; service; Traefik host; firewall host; action.
- **IP sets**, **IP set detail**, **address page**: 05a §6 (the address page shows both counters with edit/reset).

### 13.1 Counters view

Security → Counters: one row per counter record — address (link to the address page), country/ASN, ban counter,
challenge counter, current state (banned until … / challenged, `f` of `F_max`, expires … / none), last ban, last challenge,
next decrement, last source. Filters: state (banned now, challenged now, any active, all with counters), minimum ban or
challenge counter, last source (L7 rate, WAF, challenge failed, L4 meter, manual), country/ASN, last offence within a
range. Search by address or CIDR (rows inside it). Sort by any counter or time. Row actions: edit counters, reset,
ban now, challenge now, remove from dynamic sets. Selection and "all matching filter" for bulk reset. Paginated.
- **Rule sets** (`/rule-sets`, `/rule-sets/{id}`): entries ordered by drag (priorities renumbered), item editor (IP set
  picker, condition, action, auth options), attach a rule set, used-by list, warnings.
- **Service → Access tab**: root rule set editor, attached rule sets, default fallback, the flattened list with the
  origin of every item, "try a request" (address + path → trace).
- **Service → WAF tab**: mode per path, exclusions.
- **Firewall** page: global allow/block bindings (05 §4).

## 14. API

Rule sets: `/api/rule-sets` CRUD, `GET|PUT /api/rule-sets/{id}/entries`, `GET /api/rule-sets/{id}/consumers`.
Services: `GET|PUT /api/services/{id}/access` (root entries + fallback), `GET /api/services/{id}/access/flattened`,
`GET|PUT /api/services/{id}/waf`. Global rule set: `/api/rule-sets/global/entries`.
Trace: `POST /api/addresses/{address}/trace` (05a §8). Dynamic sets and addresses: 05a §8.
Counters: `GET /api/security/counters?state=&min_ban=&min_challenge=&source=&country=&asn=&since=&q=&sort=&cursor=`,
`PATCH /api/addresses/{address}/counters` (`ban_count`, `challenge_count`, `reset_failures`),
`POST /api/security/counters/reset` (addresses or a filter; returns the count).
Feeds: `POST /api/ip-sets` with `kind=feed` + feed settings, `POST /api/ip-sets/{id}/feed/preview|refresh`,
`GET /api/ip-sets/{id}/feed/runs`.
CAPTCHA: `GET|PUT /api/security/captcha/settings`, `POST /api/security/captcha/test`, `GET /api/edge-challenge/start`,
`POST /api/edge-challenge/verify`, `GET /api/edge-challenge/status`.
Dashboard: `GET /api/security/dashboard?range=&service=&traefik=&host=&action=`. Engine settings: `/api/settings/security`.

## 15. Jobs

Feed refresh; access-log tail (per Traefik host, continuous); host link (per firewall host, continuous, 05 §6); bucket
aggregation and dashboard snapshot; challenge expiry; ban expiry (database side; kernel entries expire by their own
timeout) and counter decay (both counters, record cleanup); dynamic set reconciliation (every 5 min); GeoIP update; IP set recompile on source changes.

## 16. Audit

Rule set and service access changes; IP set and binding changes (05a §9); manual dynamic actions; counter edits and
resets (single and bulk, with before/after values); feed add/remove/
enable; CAPTCHA settings and Cap secret rotation; SSO provider changes; engine settings changes.

## 17. Acceptance scenarios (each is an automated test, most also part of the S9/S10 gates)

1. Own address trips an L4 meter or the L7 rate trigger while it is in *Global firewall allow* → no drop in the kernel,
   no challenge, no ban, an event records the skip. Not allowed but banned → address page "remove / reset counter" lifts
   it on every host within one poll interval.
2. Large upload / WebDAV on a path with `headers_only` passes; the same body on an `enabled` path is inspected.
3. Flood on an SSO-protected `/admin` → challenged by A3 before the IdP is contacted; after `F_max` ignored requests
   banned in the kernel on every managed host.
4. Fallback `deny` + `trusted-devices` → `none` + `all` · `/api/public` → `none`: devices reach everything, others only
   `/api/public`, other paths `403`.
5. Two attached rule sets conflicting on `/admin` → the one with the lower priority number decides; the trace names it.
6. Home WAN address changes → the Pulse item's sets recompile; the folded `allow` on every managed host and the DNS
   records using it update without a manual step.

## 18. Tests that must exist

Unit: path normalization; flattening (nesting, priorities, disabled items, global set position, fallback); first-match
evaluation table-driven over §4.4; ban function per type incl. cap and permanent; step decay (one step per `D`, several steps at once, clock restart on
a new ban, a violation without a ban leaves the clock alone, permanent ban never decays, read-time value at boundaries); A2 windows and
exemptions; A3 transitions (solve, expiry, failures, escalation); escalation skip for allowed addresses; feed parsers per
format, dedupe, SSRF validation incl. redirects and final dial; clearance cookie signing and expiry; a valid SSO session satisfies `pass_captcha`; both counters'
increment and decay; manual counter edits (next ban uses the edited `c`, running ban unchanged); bulk reset by filter.
`TestInvariant_20_ManualAllowBeatsDynamic` (L7 side), `TestInvariant_22_SetGraphsAcyclic` (rule sets),
`TestInvariant_StageOrder_*` (Stage A always before Stage B; A1 before A2 before A3).
Integration: forward auth through a real Traefik container for every action; fail-closed vs fail-open by rule content
with Hashi stopped; Cap verify success/failure/unavailable (stub from recorded contract); WAF modes per path through the
pinned plugin; access-log tail across rotation; edge SSO against a local OIDC test provider.
E2E: build a rule set and attach it; IP set with a Pulse item used in a rule; ban now / remove; feed add with preview;
trigger and solve a challenge.
