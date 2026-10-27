# 06 — Edge security: forward auth, SSO, rules, WAF, abuse engine, CAPTCHA, blocklists

Full table definitions (subjects, states, manual entries incl. scope types `global` / `resource` / `root_domain` /
`traefik_connection` / `firewall_host`, events, buckets, blocklist tables, firewall block tracking): [06a-security-data-model.md](06a-security-data-model.md).

## 1. Overview

Traefik calls Hashi's forward-auth endpoint for every HTTP-family request on resources with a forward-auth policy.
Hashi decides allow / SSO / challenge / deny from one deterministic decision engine fed by manual entries, blocks,
blocklists, resource rules, GeoIP and rate buckets. Streams (TCP/UDP) are protected only at the firewall.

## 2. Forward-auth endpoint

`GET /api/edge-auth/forward` (Traefik `forwardAuth`, `trustForwardHeader: true` only because Traefik hosts are trusted proxies).

Input from Traefik: `X-Forwarded-Host`, `X-Forwarded-Uri`, `X-Forwarded-Method`, `X-Forwarded-Proto`, `X-Forwarded-For`,
plus `Cookie`, `Accept`, `User-Agent`.

Responses: `204` allow (with `X-Hashi-User` when SSO-authenticated, configurable auth response headers); `302` to login
or challenge for browsers; `401` auth required (API clients); `403` blocked; `429` rate-limited or challenge required
for API clients (`{"error":"challenge_required","challengeUrl":…}`).

Resource resolution uses host **and** path routing exactly as Traefik does (most specific route wins), not "first host
match".

### 2.1 Policy modes per resource

| Mode | Behaviour |
| --- | --- |
| Off | No forward-auth middleware attached |
| SSO required | Fail **closed** if Hashi cannot evaluate |
| Adaptive (default for HTTP resources) | Anonymous allowed normally; SSO and/or challenge required during elevated abuse state or by rules |
| Observe | Decisions recorded, never enforced |

Unreachable-Hashi behaviour: Traefik cannot "fail open" natively, so every Traefik host gets a tiny **local fallback**:
forward auth targets Hashi via a Traefik service with a health check and a failover to a static `204` responder
**only** for resources in Adaptive/Observe mode with no active challenge/block policy. SSO-required resources have no
fallback (fail closed). Verify this construct against the pinned Traefik version before implementation.

### 2.2 Performance budget

p99 ≤ 5 ms in-process for cache hits. Decision inputs (manual entries, blocks, blocklist entries as a radix tree, rules,
GeoIP reader) live in memory, refreshed on change notifications and every 30 s. Event recording is async through a
bounded channel; on overflow, events are sampled and a counter records drops — the decision is never delayed by logging.

## 3. Decision engine

### 3.1 Effective decision order

1. Invalid/untrusted request metadata → deny.
2. Manual block.
3. Firewall block (subject is in an active firewall block).
4. High-confidence blocklist entry with firewall enforcement.
5. Soft block.
6. Active challenge required.
7. Resource-specific rule (priority order).
8. Manual allow.
9. Default resource behaviour (policy mode, rate thresholds).

Manual allow prevents Hashi-controlled blocking and escalation but **does not bypass SSO or CAPTCHA** in this version.
Manual allow entries carry explicit flags: `bypassBlocking` (default true), `bypassAdaptiveEscalation` (true),
`bypassRateLimit` (false), `bypassChallenge` (false), `bypassSso` (false). Only the first two are editable initially;
the UI shows all five with their meaning. Block entries must have all bypass flags false.

### 3.2 Rule actions and inputs

Actions: allow, deny, require SSO, require challenge, soft block, firewall block, bypass blocking (via manual allow).
Inputs: subject state, resource security profile, request bucket state, manual entries, blocklist matches, GeoIP/ASN,
challenge state, requests-while-challenged, failed challenges, previous escalations.

Every decision is explainable: the engine returns the decision plus the ordered list of evaluated steps and the one that
decided. The subject detail page shows this trace.

### 3.3 Forward-auth evaluation order

Parse metadata → resolve resource → resolve subject(s) (IP, CIDR matches, ASN, country, region, session) → load manual
entries → load block/challenge state → blocklist matches → resource rules → rate buckets → decide → record event → respond.
Upstreams never receive requests while the subject is challenged or blocked.

## 4. Edge SSO

- One or more OIDC providers (issuer discovery, client ID, client secret in vault, scopes, claim mapping for username/email/groups).
- Per resource: default provider or explicit provider; optional allowed users/groups.
- With a default provider Hashi redirects straight to it (no chooser page).
- Edge session cookie on the root domain (e.g. `.example.com`) so one login covers all subdomains; HMAC-signed session ID,
  server-side `edge_sessions` row; settings: max session length, idle timeout, remember-device.
- Flow: `/api/edge-auth/login?rd=<url>` → IdP (PKCE + state + nonce) → `/api/edge-auth/callback` → validate `rd` is a
  managed domain (no open redirect) → set cookie → redirect. `/api/edge-auth/logout` clears and optionally calls IdP end-session.
- Authenticated SSO sessions get higher rate thresholds and skip adaptive challenge for that session, but never bypass
  WAF, explicit blocks or impossible-volume protection.

## 5. Security subjects and telemetry

### 5.1 Subjects

Types: `ip`, `cidr`, `asn`, `country`, `region`, `session`, `composite` (e.g. IP+resource, IP+root domain for buckets).
`security_subjects`: type, value, normalized value (unique with type), first/last seen, last country/region/ASN/AS org,
current state, metadata.

States: observed, warm, suspect, challenged, soft blocked, firewall blocked, manually allowed, manually blocked.

`security_subject_states`: challenge_required (+since, reason, resource, attempts), requests_while_challenged,
failed/successful challenge counts, last solve, soft_blocked_until, firewall_blocked_until, manual allow/block active,
last escalation reason/time.

### 5.2 Signals and ingestion

Signals: forward-auth decisions (primary, in-process), Traefik access logs, Traefik service logs, firewall counters and
sampled drop logs, WAF events, manual entries.

Access-log ingestion: a job tails `/var/log/hashi/traefik/access.log` on every Traefik host over SSH every 60 s using a
stored (inode, byte offset) cursor (`tail -c +<offset>`), handles rotation (inode change → read rest of old file from
`.1`, then new file), bounded per read (e.g. 8 MB), parses JSON lines, and aggregates. WAF events are taken from the
Coraza plugin's log lines in the same stream.

### 5.3 Buckets

`security_buckets_1m`: bucket start, subject type + normalized value, resource, root domain, Traefik connection,
country, region, ASN, method, path prefix, status class, request/blocked/challenged/challenge-ignored/failed-challenge counts.
Indexed for subject + recent window lookups. Aggregation happens in memory and flushes closed minutes.

### 5.4 Events

`security_events`: occurred_at, subject type/value/normalized, resource, connection, event_type, severity, decision,
source, reason, method, path, status, user_agent_hash, request_id, metadata. Indexes on (normalized subject, time),
(resource, time), (event_type, time), (severity, time).

## 6. Adaptive path and escalation

Default path:

1. High traffic → `suspect`.
2. If the resource supports adaptive auth → require SSO or challenge (resource profile decides).
3. Successful SSO → higher thresholds for that session; WAF stays active.
4. Continued anonymous excess or WAF/rate violations → soft block at the edge.
5. Clearly malicious IPs → firewall block set.

### 6.1 Ban duration policies

Types (no executable expressions): constant, linear (`base × n`), exponential (`base × m^(n−1)`), capped exponential,
permanent after N. Fields: base_duration_seconds, linear_multiplier, exponential_multiplier, max_duration_seconds,
permanent_after_count, count_window_seconds, reset_count_after_seconds.

Offense counts are tracked separately from active state and **never** reset by a CAPTCHA solve (soft/firewall block
counts, manual block history, events and repeat-offender counters persist).

### 6.2 Configurable defaults (Settings → Security; resource profiles may override)

Challenge trigger thresholds, challenge-ignored thresholds, soft block policy, firewall block policy, repeat-offender
policy, CAPTCHA success resets or decays the triggering bucket (+ decay %), minimum time between repeated challenges,
max challenge failures before escalation, max requests while challenged before escalation.

Suggested initial values: challenge-ignored = 30 protected hits within 5 min → soft block 10 min; firewall block at 100
hits within 5 min while challenged or repeated ignored violations; 5 failed verifies in 10 min → soft block; repeated
challenge/solve/abuse cycles use the ban multiplier.

## 7. CAPTCHA (Cap)

Hashi integrates with an **existing** Cap Standalone instance; it does not deploy Cap.

Setup/settings: enable, Cap public challenge base URL, site key (visible), secret key (service secret), verification
timeout, instrumentation/headless detection expectation, public challenge resource domain, optional Cap admin resource
domain (+ recommend SSO), test verification.

### 7.1 Resources

- **Public challenge resource** (system, required while CAPTCHA is enabled; edit only via CAPTCHA settings): no SSO, no
  CAPTCHA, not subject to adaptive rules, still subject to hard firewall blocks; exposes only the paths the challenge
  flow needs; never the Cap admin dashboard or management API; uses a real reverse-proxy domain (never `hashi.home.arpa`).
- **Cap admin resource** (optional, normal, user-deletable): strongly recommend SSO + admin-only; no CAPTCHA by default
  (dependency loop); normal rate limiting allowed.

### 7.2 Challenge semantics

CAPTCHA is a middle state, not a whitelist. On trigger: set `challenge_required`, record reason + triggering buckets;
protected resources redirect (browser) or return `429 challenge_required` (API); upstreams not contacted;
requests-while-challenged counted; challenge resource stays reachable; firewall blocks still win.

On solve: verify token with Cap server-side; record success; clear `challenge_required`, `requests_while_challenged`,
`failed_challenge_count`; reset or decay only the triggering buckets; do **not** clear offense history, bypass SSO, or
grant a clearance window (no default "free pass").

Tracked while challenged: requests_while_challenged, resources_hit_while_challenged, captcha_page_requests,
captcha_verify_attempts, failed_challenge_count, challenge_ignored_count. Escalation per §6.2.

Browser vs API detection: `Accept` contains `text/html` and method is GET/HEAD → browser redirect to
`/api/edge-challenge/start?rd=…` preserving the original URL (unsafe/cross-origin `rd` → resource root); otherwise API response.

Challenge page: minimal, only required assets, no SSO, no protected API calls, shows the domain being accessed, submits
the Cap token to Hashi (`POST /api/edge-challenge/verify`), handles failure without leaking details, never exposes the
Cap secret.

## 8. Blocks

| Block type | Enforcement |
| --- | --- |
| IP / CIDR | forward auth + Traefik where practical + firewall `hashi_blocked[6]` (if firewall-enforced) |
| ASN | forward auth only (never expanded to firewall ranges by default) |
| Country / region | forward auth only |

Block entry: scope, reason, source (manual / engine / blocklist), created by, created at, expiry or permanent, last hit,
applied-to host list with per-host state, last apply result/error.

Firewall-affecting changes always go through a firewall sync plan with preview — never applied directly from a button.

## 9. Blocklists

- Offered individually in setup and in Security settings; **all disabled until explicitly selected**; warning about false positives.
- Recommended feeds: Feodo Tracker botnet C2 IPs, Spamhaus DROP, Spamhaus DROPv6, DShield recommended block list, FireHOL Level 1. Plus custom URL.
- Each shows: name, URL, description, false-positive risk, update frequency, format, enforcement mode, firewall-capable,
  licence/user-responsibility note.
- Formats: plain IP/CIDR lines, with comments, FireHOL `.ipset`/`.netset`, CSV/TSV (configurable column), JSON array of
  strings, JSON object with configured array field. Out of scope: Suricata/Snort rules, domain/URL lists, complex threat-intel schemas.
- Parsing: normalize IPv4/IPv6/CIDR, reject invalid, dedupe, keep source attribution and line numbers, record errors and
  ignored entries, preview before enabling, never partially apply a failed update.
- Fetcher (SSRF-safe): HTTPS only by default (HTTP only with explicit warning), TLS verification, max redirects, max size,
  timeout, Hashi user agent, ETag/Last-Modified. Validate the URL, every DNS answer, every redirect target **and the final
  dialed IP** (custom `net.Dialer.Control` hook) against denied ranges: loopback, RFC1918, link-local, multicast, ULA,
  IPv6 link-local, cloud metadata ranges, Docker bridge ranges (unless dev mode).
- Storage: `blocklist_sources`, `blocklist_entries` (enforcement mode: middleware / firewall / both), `blocklist_fetch_runs`
  (counts parsed/added/removed/unchanged/rejected, content hash).
- Failure keeps last-known-good entries, marks the source degraded, shows last success and error. Disabling a source
  removes its entries from decisions and queues a firewall plan if needed; history is kept.

## 10. WAF

Coraza-based Traefik WAF plugin with OWASP CRS, free, local, pinned version (verified to exist), loading the full CRS
rule set. Per-resource mode: off / detect / block. Global default: detect
during setup validation; block for new internet-exposed resources after the user accepts the default security profile.
Per-resource exclusions (rule IDs, paths) for false positives. WAF matches become security events. Hashi never implements a WAF engine.

## 11. Security UI

### 11.1 Dashboard (Security tab)

Widgets: allowed / blocked / challenged requests; WAF detections and blocks; active firewall blocks; top 10 blocked IPs
(count, last seen, country, ASN, reason, expiry); top 10 blocked countries; top 10 blocked ASNs; top 10 resources
receiving blocked/challenged traffic; top challenged IPs; recent manual actions; blocklist matches over time; CAPTCHA
solved/failed/ignored; active soft blocks; stale blocklist sources; stale GeoIP database; recent security events.

Filters: 1 h / 24 h / 7 d / 30 d; resource; Traefik instance; firewall host; action (allowed, challenged, blocked, WAF-detected).
Reads the same buckets and tables as the engine — no separate analytics store.

### 11.2 Search and subject detail

Global search accepts IP, CIDR, ASN number, country code, region, and text over recent events. Result rows: subject,
state, last seen, country/ASN, active manual entries, blocklist hits, active challenge/block, last action.

Subject detail shows: effective decision + trace, matching manual entries, blocklist entries, resource rules, adaptive /
challenge / soft-block / firewall-block state, ban and challenge history, recent events, bucket summary, firewall
application state, and actions: add/remove manual allow/block, make permanent, shorten/extend/expire, escalate soft →
firewall (IP/CIDR only), de-escalate, preview firewall sync, view affected resources, audit trail. Timeline filterable by
event type and resource, including forward-auth decisions, request summaries, WAF, rate triggers, challenge
required/solved/failed/ignored, soft/firewall block created/applied/expired, manual changes, blocklist hits, GeoIP/ASN changes.

## 12. GeoIP

MaxMind GeoLite2 Country + ASN (account ID + licence key in vault; optional City/region if the user's licence allows).
Download job with checksum, atomic swap of the reader, staleness warning after 14 days. Without databases,
country/region/ASN rules are invalid and widgets show "GeoIP unavailable".

## 13. API

Subjects: `GET /api/security/subjects/search?q=`, `GET /api/security/subjects/{id}`, `/events`, `/buckets`, `/effective-decision`.
Manual entries: `POST /api/security/manual-entries`, `PATCH|DELETE /api/security/manual-entries/{id}`, `POST …/{id}/expire`.
Blocks: `POST /api/security/blocks`, `PATCH /api/security/blocks/{id}`, `POST …/{id}/extend|shorten|make-permanent|expire|preview-firewall-sync`.
Blocklists: `GET|POST /api/security/blocklists`, `GET|PATCH|DELETE /api/security/blocklists/{id}`,
`POST …/{id}/fetch-preview|enable|disable|refresh`, `GET …/{id}/runs|entries`.
CAPTCHA: `GET|PUT /api/security/captcha/settings`, `POST /api/security/captcha/test`, `GET /api/edge-challenge/start`,
`POST /api/edge-challenge/verify`, `GET /api/edge-challenge/status`.
Dashboard: `GET /api/security/dashboard?range=&resource=&traefik=&host=&action=`.
Policies: `GET|PUT /api/security/policies`.

## 14. Jobs

Blocklist fetch; security bucket aggregation (+ dashboard snapshot, partition expiry); block expiry (soft and firewall,
queue firewall plan, expiry events); challenge cleanup (expire stale challenges, abandoned attempts, max-duration reset,
history preserved); GeoIP update; access-log ingestion.

## 15. Audit (in addition to global rules)

Manual allow/block create/update/delete; firewall block apply/remove; blocklist source create/update/delete/enable/
disable; CAPTCHA settings; Cap secret rotation; internal DNS domain change; agent-bound target change; any high-risk apply.

## 16. Tests that must exist

Unit: IP/CIDR normalization (incl. mapped/special), ASN/country/region matching, decision order (table-driven, every
step), manual allow defaults and limits, ban policy math, blocklist parsers per format, dedupe, SSRF validation incl.
redirects and final-dial checks, challenge state transitions, requests-while-challenged escalation, solve reset/decay.
Integration: forward-auth allow/SSO/challenge/block through a real Traefik container; Cap verify success/failure/unavailable
(Cap stub from recorded contract); blocklist fetch → store → firewall plan; edge SSO against a local OIDC test provider.
E2E: search IP → timeline; manual block/allow; extend/shorten; enable blocklist in setup; custom URL; configure Cap;
trigger and solve challenge.
