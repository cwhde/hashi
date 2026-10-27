# Hashi — before and after (full comparison)

Compares every section of the four V2 documents — base spec (B), security/agent/CI addendum (A1), admin-session
addendum (A2), UI design-foundation addendum (A3) — with the new spec in `docs/spec/`.

Legend: **Same** = behaviour unchanged (wording may be condensed) · **Changed** · **Added** · **Removed** ·
**Restored** = I had condensed it away in the first draft and put it back after this review.

Part 4 lists the production setup today vs after Hashi. Part 5 answers "are the old bugs still prevented?".

---

## 1. Base specification (B)

| B § | Topic | Status | Summary |
| --- | --- | --- | --- |
| 1 | Purpose | Same | Same product scope and goal. |
| 2 | Product principles | Same + Added | All six kept; added a seventh, "proven against reality" (a feature counts only after it worked on the real systems). |
| 3 | Non-negotiable rules 1–24 | Same, reorganized | Became 19 numbered invariants in README. Merged pairs: rules 7+8+9 (plan/apply, atomic writes, no identical rewrites), 12+13 (minimum counts). Rule 20 "Plan, Apply, Reconcile" renamed **Plan, Apply, Verify** (same meaning). Rules 15 (one UI component system), 16 (one DB) and 23 (CI must pass linters/scans) moved to 01-platform and 11-delivery instead of the invariant list. Added invariant 19: public repo, no secrets or private inventory committed. |
| 4 | Tech stack — backend | **Changed** | .NET 10 / C# 14 / ASP.NET Minimal APIs / EF Core 10 / PostgreSQL 18 / FluentValidation / Fido2NetLib / ASP.NET OIDC / SSH.NET / YamlDotNet / Cronos / Serilog → Go / net/http / sqlc / SQLite (WAL) / goose migrations / go-webauthn / go-oidc / x/crypto/ssh + pkg/sftp / yaml.v3 strict / robfig/cron / slog with redacting handler. Reason: one language with Pulse, one binary, one container, no DB server. |
| 4 | Tech stack — API contract | **Changed** | Old: OpenAPI generated from the C# code. New: spec-first `api/openapi.yaml`; server stubs and the TS client are generated from it; CI fails if generated code is stale; every endpoint declares its scope and reauth requirement in the spec. |
| 4 | Tech stack — frontend | Same (version fix) | Same libraries (shadcn-svelte/Bits UI, Tailwind v4, Lucide, uPlot, CodeMirror 6, Zod, Vitest, Playwright). "SvelteKit 5" corrected to Svelte 5 + SvelteKit 2. Now built as a static SPA embedded in the Go binary. Package manager pinned to pnpm. |
| 4 | Deployment | **Changed** | Two containers (Hashi + Postgres) → one. `/data` required; `/logs` optional (**Restored**). Ports 8080/8081/8082 stay defaults but are configurable (the real Docker host already uses them, so deploy uses 3340–3342). Multi-arch amd64/arm64 kept. |
| 4 | Pulse agent | Same | Go, static, distroless < 10 MB, one-line installer, Docker snippet. Token stored as HMAC with a server pepper instead of a plain hash (stronger, same behaviour). |
| 5 | Architecture | Same + Added | Same control-plane/data-plane split. Added: one process with three listeners; forward auth must not depend on the job runner (in-memory decision caches). |
| 6 | Connections | Same + Added | Same types incl. optional NetBird API. Added Cap as a connection type. Health states and deletion policy (**Restored**). Added reusable connection target model from A1 §9. |
| 6 | Resources | Same | All fields kept. Added explicit meaning of `http` / `https` / `h2c` kinds (what the upstream speaks). |
| 6 | Resource rule model | Same (merged with A1) | Base actions (bypass auth, block, pass to auth, adaptive challenge) merged with A1's list (allow, deny, require SSO, require challenge, soft block, firewall block, bypass blocking). Match types unchanged. GeoIP-missing → rule invalid, unchanged. |
| 7.1 | Bootstrap access | Same + Added | Same flow and CIDR list. Added: plain-HTTP bootstrap from a private IP gets a non-`Secure` bootstrap cookie (HTTPS cookies would not work there); CLI `hashi bootstrap reset`; credentials survive restart. |
| 7.2 | Base settings | Same | Added validation rules (FQDN under root domain, absolute internal URL, no hardcoded port). |
| 7.3 | DNS provider setup | Same + clarified | Same fields, read-only validation, `_hashi-test` probe only with approval, import table, NS/SOA never pruned. Clarified: exactly one zone — the root domain's zone is picked automatically, or the user chooses from the zones the token sees. No multi-zone (never was). |
| 7.4 | Certificate provider | Same + Added | Google Trust Services + EAB kept. Added Let's Encrypt as an optional second directory. DNS-01 mechanism changed: see B §10.2. |
| 7.5–7.6 | Traefik / firewall setup | Same + Added | Same fields and validations. Added mandatory SSH host-key pinning (fingerprint shown on first contact). Firewall setup now first captures the existing ruleset. |
| 7.7 | System resource | Same | — |
| 7.8 | Passkey + vault setup | **Changed** | Old: after HTTPS the user logs in with bootstrap again, registers a passkey, PRF wraps the vault root key, recovery key mandatory, test unlock. New: HTTPS reachability verified with a server-side nonce check; passkey + recovery key (confirmed by re-entering a checksum segment) + test unlock. PRF moved to optional strict mode (see B §8). |
| 7.9 | Optional steps | Same + merged | OIDC, AdGuard, notifications, MaxMind, widgets kept; A1's Cap, blocklists and internal agent DNS steps merged in. |
| 8 | Secrets vault | **Changed** | Old: three classes (session-unlocked, service-sync, server-operational); admin vault wrapped by passkey PRF + recovery key; service vault wrapped by a Docker secret. New: one master key from a Docker secret; per-secret AES-256-GCM DEKs wrapped by class keys derived from it (service / admin-only / operational); recovery key wraps a copy of the master key for restore. "Passkey-only" protection survives as opt-in **strict mode**: no key on disk, unlock after restart with a PRF passkey or the recovery key. Same trade-off explained in setup. Added: secrets are a Go type that can never be logged or serialized. |
| 9 | Admin auth | Same + Added | Passkeys, multiple passkeys, recovery key for vault, secure cookies, CSRF, reauth list kept. Session timeouts are now governed by A2 (4 h idle / 8 h absolute). Added: console-only recovery login (`hashi admin recover`) to register a new passkey if all are lost; cannot delete the last passkey. |
| 9 | Edge SSO | Same + Added | Same. Added flow details (PKCE, state, nonce, open-redirect check on `rd`). |
| 10.1 | Traefik install + updates | **Changed** | Old: detect package manager, install Traefik if absent. New: install the official release binary with checksum verification. Because no package manager is involved, **Hashi now does the updating**: daily release check, optional auto-apply of patch releases, manual minor/major upgrades as a high-risk plan, one host at a time, automatic rollback to the previous binary on failure, config validated against the new version first. |
| 10.2 | Static config | Same + **Changed** ACME | Entry points, file provider, access/general JSON logs, ping, dashboard not public, pinned plugins kept. **Changed:** DNS-01 now via lego's `httpreq` provider calling Hashi, which writes the TXT record through its own DNS module — Traefik hosts no longer hold the DNS token, and any DNS provider module works. Added: Traefik API bound to localhost for verification; `forwardedHeaders.trustedIPs` so client IPs survive DNAT; optional HTTP/3; access-log header dropping specified. |
| 10.3 | Dynamic files | Same | Same six files, same names (**Restored** — I had renamed two without reason). |
| 10.4–10.6 | Middlewares, sync rules, TCP/UDP limits | Same + Added | Same. Added: reused ports pending removal are cancelled, not deleted; apply verifies through Traefik's API and restores the previous static config if a restart fails. |
| 11 | Forward auth | Same + Added | Same endpoint, inputs, responses, four policy modes. Added: concrete fail-open mechanism (Traefik failover to a local `204` only for adaptive/observe resources; SSO-required stays fail-closed) — marked "verify against the pinned Traefik first"; p99 ≤ 5 ms budget; resource resolution uses host **and** path. |
| 12 | WAF | Same | Coraza + full OWASP CRS, modes, exclusions, events. |
| 13 | Abuse detection | Same + **Changed** storage | Signals, dimensions, states, default path, block types kept. PostgreSQL partitioning → minute buckets in SQLite with batched retention. Added how Traefik access logs are collected: SSH incremental tail with inode/offset cursor every 60 s (old spec never said how). |
| 14.1–14.2 | Firewall host model, host DNS | Same + Added | Same fields and `host` / `via.` / `on.` records. **Added:** `external_nat` edge mode for hosts behind a router (your home server). |
| 14.3 | Firewall script | Same + Added | Same paths, ipsets, chains, behaviours. Added: atomic build via temporary ipsets + `ipset swap` and `iptables-restore --noflush`; explicit table per chain; IPv6 set `hashi_blocked6`; rollback script; packages are installed by the plan. Added: Hashi's first apply removes an existing firewall script (its chains, ipsets, cron). Loopback DNAT, corosync MTU routes and `/etc/hosts` pins from your current script were considered and **not** adopted. |
| 14.4–14.5 | NetBird, safety | Same + Added | Same modes, detection, preserve-list and safety rules. Rescue rollback made concrete (save → write executable rollback → arm timer → apply → verify via a **new** SSH connection + NetBird status → disarm). |
| 15 | DNS provider | **Changed** provider, Same model | Interface kept (+ Verify; RRSet is the unit). Hetzner legacy DNS API → Hetzner Cloud API (the old one is shut down). Adapter must be built from recorded real responses. One zone only. Ownership: DB authoritative (same); old optional "ownership marker TXT records" became an optional provider capability (Hetzner implements it with RRSet labels; other providers use comments/tags or skip it) — provider stays a swappable module. Added: provider-protected RRSets are read-only; conflict items instead of overwrites; CNAME exclusivity; **apex flattening** for root-domain resources (CNAME at apex is impossible). |
| 16 | AdGuard | Same + Added | Same rules. Added: every AdGuard instance is written directly (replaces adguardhome-sync); disabled connections untouched; duplicate cleanup only when previewed. |
| 17 | Pulse | Same + Added | Same model, token rules, heartbeat, resource use. Added two small things: (1) if a service's *forwarding target* is "agent X's IP" and that IP changes, Hashi also updates the Traefik config, not only DNS; (2) optional reachability check — Hashi tries to connect to the agent's reported address so the UI shows whether Hashi can actually reach it. |
| 18 | Monitoring | Same + Added | Sources (+ Pulse), check types, states, storage, retention, views, UI kept. Added: UDP check only with a configured probe/response (silence ≠ healthy); TLS expiry thresholds 21 d / 7 d; fixed safe probe order for DNS entries; public-path vs internal-path checks to tell "service down" from "edge down"; failure/recovery thresholds; rollups only from closed buckets. Tables `monitor_rollups_1m/5m/1h` merged into one table with a bucket-size column. |
| 18.6 | Notifications | Same + Added | SMTP/Telegram/Discord, routing, pairing flows kept. Added delivery log with retries; pending high-risk plans and vault lock can notify. |
| 19 | Security dashboard | Same (merged with A1) | All widgets and filters kept, merged with A1 §11.2. |
| 20 | Public app dashboard | Same + Added | Same. Added: failed refresh shows a "stale since" banner. |
| 21 | Admin IA | Same | Same 13 navigation items, rail behaviour and overview widgets. Grouping and page layouts are now designed in the UI design slice S0b (I had invented a grouping; removed). Activity live log view (**Restored** from V1 lessons, B §34). |
| 22 | Visual design | Superseded (as before) | Shades of Purple was already superseded by A3. Assets from `static.juzo.io` kept; backgrounds optional and off by default (**Restored** — first draft limited them to public pages). |
| 23 | Scripts | Same + Added | Same fields and safety. Added: systemd timers preferred with a cron-equivalence test; a run wrapper so scheduled runs get the same env, timeout, redaction and history as manual runs; an all-disabled target set means no hosts. |
| 24 | Settings | Same + Added | Same categories + internal agent DNS, CAPTCHA, trusted proxies, vault mode. Added rule: every setting must have a runtime consumer and a test. |
| 25 | Sync engine | Same + Added | Same preview → apply → verify flow. Added, in plain words: (1) a table saying which changes are applied automatically (e.g. updating a Hashi-owned record), which need your approval (deletes, opening ports, firewall changes) and which are never allowed (NS/SOA, touching things Hashi doesn't own); (2) only one change runs against the same system at a time, whatever started it; (3) if the real state changed between preview and apply, Hashi previews again and asks again when something risky changed; (4) what gets applied is exactly what you saw in the preview; (5) a fixed order for a full sync (DNS → Traefik → firewall → AdGuard → monitoring). |
| 26 | Database schema | **Changed** | Same domains; adapted to SQLite: ULID text IDs, Unix-ms timestamps (`_at` instead of `_at_utc`), JSON text instead of `jsonb`. Renamed/merged: `system_resources` → origin column; `vault_wrapped_keys` → inside `secret_records`; `sync_steps`/`sync_diffs` → `sync_plan_items`/`sync_runs`; `traefik_generated_files` → `traefik_files`; firewall tables renamed. Added `admin_sessions`, `connection_targets`, `pulse_heartbeats` retention. Full security tables: 06a (**Restored** verbatim). |
| 27 | API surface | Same + Added | All groups kept. Added `/api/acme/httpreq/*`, `/api/events` (SSE), explicit reauth endpoints, RFC 9457 errors, cursor pagination, `202` + run ID for long operations. |
| 28 | Repository structure | **Changed** | .NET solution layout → Go layout (`cmd/`, `internal/<area>/`, `api/`, `web/`, `agents/pulse/`, `testdata/contracts/`). V1 at `legacy/v1/` instead of `hashi.old/`. |
| 29 | Development workflow | Same + Added | Same practices. Added: max ~500-line PRs, spec changes in separate commits, reproduction-first bug fixes with a two-attempt stop. |
| 30 | CI/CD | **Changed** tools | Same three workflows and scans (gitleaks, Semgrep, Trivy fs + image). `dotnet format/list` → gofmt/staticcheck/golangci-lint/govulncheck; `pnpm audit` → osv-scanner. Registry and tags unchanged. |
| 31 | Test strategy | Same + Added | All unit/integration/E2E/safety tests kept. Added contract replay tests, integration against real binaries in containers (Traefik, sshd, iptables netns, AdGuard), visual tests, invariant tests named `TestInvariant_*`. |
| 32 | Implementation plan | **Changed** | 14 phases built in parallel → 13 vertical slices built one at a time, each gated on working against the real servers (prod-proof in `docs/progress.md`). Phase −1 (move V1) done as `legacy/v1/`. |
| 33 | Operations | Same + **Changed** | PostgreSQL backup → nightly `VACUUM INTO` + off-host copy of `/data`. DR and locked behaviour same. Added: automatic pre-migration backup, `/metrics`. |
| 34 | V1 lessons | Same | Topology mapping, sync history, live logs (**Restored**), config masking, monitor generation, no identical writes — all present. Plus a concrete V1 cutover plan. |
| 35 | Reference links | Same (**Restored**) | Kept; dead Hetzner DNS link replaced, .NET/Postgres links dropped, Go library links added. |

## 2. Security / agent / CI addendum (A1)

| A1 § | Topic | Status | Summary |
| --- | --- | --- | --- |
| 1–4 | Purpose, non-goals, terminology, compatibility | Same | All non-goals kept (Pulse stays Go, Hashi doesn't deploy Cap, `hashi.home.arpa` is not a proxy domain, manual allow doesn't bypass SSO/CAPTCHA, no auto-enabled blocklists, arm64 stays, no runner config in spec). Terminology folded into 06. |
| 5.1–5.4 | Subject search, decision order, detail actions, timeline | Same | Same 13 detail items, same 9-step order, same bypass flags and defaults, same actions and timeline events. Added: every decision returns its evaluation trace. |
| 5.5 | Data model | Same (**Restored**) | First draft only named the tables; full field lists, constraints and indexes now in 06a verbatim. |
| 6 | Ban policies, offense counting, settings | Same | All five policy types, fields, no-eval rule, offense history never reset by CAPTCHA, all settings and suggested defaults. |
| 7 | CAPTCHA (Cap) | Same | All semantics kept (no free pass, requests-while-challenged escalation, browser vs API, challenge page rules, rule-engine action). Cap stays the provider. Added a concrete browser/API detection rule. |
| 8 | Blocklists | Same + Added | Feeds, formats, parsing, enforcement, staleness kept; storage fields in 06a (**Restored**). Added: SSRF check also on the actually dialed IP and after normalizing IPv4-mapped addresses. |
| 9 | Agent-bound connections | Same | All fields, IP modes, validation, change behaviour. |
| 10 | Internal agent DNS | Same | Same rules, normalization, settings, stale behaviour. Added: deleting owned agent rewrites is high-risk unless caused by deleting the agent. |
| 11 | CAPTCHA / security UI | Same | Merged into 06 §11. |
| 12 | API surface | Same | All endpoints kept (written in compact `…/{id}/extend|shorten` form). |
| 13 | Background jobs | Same + Added | All five jobs + GeoIP update and access-log ingestion. |
| 14 | Forward-auth decision flow | Same | Same 11 steps and decision actions. |
| 15 | Firewall integration | Same (**Restored**) | Now verbatim in 06a. |
| 16 | Security and privacy | Same | Never-log list, audit list, reauth list, SSRF checks. |
| 17 | CI/CD optimization | Same + **Changed** | Cross-build, caches, scan-the-digest, no duplicate builds, cache misses don't fail — kept. .NET-specific items replaced by Go cross-compilation (`GOARCH`, no emulation needed). **Changed:** legacy V1 image is kept multi-arch only until cutover, then deleted with `legacy/v1/` (old text required it permanently). |
| 18 | Tests | Same | Condensed into 06 §16 and 11-delivery; Dockerfile/workflow assertion checks kept. |
| 19 | Migration plan A–G | Removed (moot) | It described migrating an existing V2 database; there is none. Its content lives in the slices. |
| 20 | Acceptance criteria | Same (distributed) | Each criterion is a requirement in 06/03/08/11 rather than one numbered list. |

## 3. Admin session addendum (A2) and UI addendum (A3)

| § | Topic | Status | Summary |
| --- | --- | --- | --- |
| A2 1–12 | Model, issuance, IP binding, trusted proxies, timeouts, scopes, reauth, revocation | Same + Added | All kept exactly (strict per-token IP binding, 4 h idle / 8 h absolute, 5-min reauth, 8 scopes, revocation). Added: DB stores only `sha256(session id)`; `setup.*` bootstrap scopes; trusted-proxy resolver shared with every other IP check. **Changed:** cookie name `hashi.session` → `__Host-hashi.session` (browser-enforced Secure/path binding). |
| A2 13–15 | API, audit, cleanup | Same | — |
| A2 16 | Migration | Removed (moot) | No old cookies exist. |
| A2 17–18 | Tests, acceptance | Same (**Restored**) | Verbatim in 02 §6. |
| A3 1–4 | Purpose, scope, philosophy | Same (as intent) | Your design intent kept: calm, rounded, dense-not-cramped, overview before detail, quiet surfaces, immediate feedback. |
| A3 5.1–5.10 | Reference descriptions, rejected direction | Same | Kept as written descriptions of your screenshots (10a); size estimates marked indicative. The screenshots themselves were never in the repo. |
| A3 5.11–5.13, 6–8, 11–22 | Pixel targets, density budget, radius/spacing scales, grid, typography sizes, surfaces, panel/table/form/button recipes, motion durations, chart geometry, responsive/a11y/performance rules | **Removed** (rules) / kept as quality bar | These were the earlier agent's own rule sets. Replaced by: a short quality bar (WCAG 2.2 AA, keyboard, reduced motion, mobile usable, no theme flash, tokens only) and the new UI design slice S0b, which designs layouts and component rules for **all features at once** before feature pages are built. |
| A3 9–10 | Four themes, colours, custom themes | Same | Theme colour table identical; Graphite default; custom themes kept as a feature (preview, reset, contrast check, import/export). |
| A3 23 | Frontend stack, CSS examples, "required changes from current frontend", primitives list, validation scenes A–E | Stack Same; rest **Removed** | Stack unchanged (shadcn-svelte/Bits UI, Tailwind v4, Lucide, uPlot, CodeMirror). Code samples, the fix-list for V2's UI code, the primitives list and the five validation scenes were agent-written specifics; S0b defines its own. |
| A3 24–26 | Tests, implementation sequence, acceptance | **Changed** | Visual tests kept (key pages × themes × widths). Sequence: full UI design first (S0b), custom theme editor in S11. |
| — | UI design before features | **Added** | Slice S0b: page inventory for every feature, navigation, layout/component rules, tokens for four themes, every page as a clickable mock in the SPA with mock data — approved by you before feature work. |

## 4. Production today vs after Hashi

| Function | Today | After Hashi |
| --- | --- | --- |
| Public DNS | V1 Hashi writes CNAMEs every 5 min from a topology TXT record + Pangolin's resource list | Hashi from its own resources and host records; TXT record retired |
| Reverse proxy (VPS) | Traefik container inside the Pangolin LXC, routes from Pangolin, badger auth | Hashi-installed Traefik on a fresh LXC with Hashi config; Pangolin and its Traefik deleted |
| Reverse proxy (home) | Native Traefik on Alpine, copy of Pangolin's static config, routes pulled from Pangolin | Hashi-installed Traefik with Hashi config; old instance removed |
| Edge auth | Pangolin badger plugin | Hashi forward auth + edge SSO (Pocket ID as IdP) |
| Firewall (VPS) | `/scripts/firewall.sh` via cron every 15 min; ~30 duplicate FORWARD rules accumulated | Hashi-generated rules in Hashi chains, check-before-insert, rescue timer; old chains/cron removed in first apply |
| Firewall (home) | None on host; the home router does NAT and runs NetBird; traffic enters via the VPS | `external_nat` mode: no firewall management on the home host; routing via VPS as today |
| Internal DNS | 2 AdGuard instances + adguardhome-sync every 10 min | Hashi writes rewrites to both; adguardhome-sync retired |
| Monitoring | Gatus, 41 endpoints generated by V1 | Built-in monitoring + public status page; Gatus retired |
| App dashboard | — | Public app dashboard |
| Abuse / WAF / blocklists | CrowdSec log path set on home Traefik, no CrowdSec process found | Hashi security engine, Coraza WAF, CAPTCHA, blocklists |
| CAPTCHA | — (Cap per the old spec) | Cap, integrated (Hashi doesn't deploy it) |

## 5. Are the old V2 bugs still prevented?

Yes. I removed only the "(V2 bug AUD-…)" annotations; the requirement each annotation explained is still in the spec as
a normal rule, e.g.:

- Hetzner Cloud API only; adapters built from recorded real responses (03 §2).
- No `traefik check`; verify through Traefik's API (04 §6). Static config changes really restart Traefik and roll back on failure.
- NAT chains in the `nat` table; DNAT to the Traefik entry-point port; block sets also on FORWARD; NetBird masquerade in `nat` (05 §3.1).
- Rescue rollback script executable before arming; timer disarmed only after a new SSH connection works (05 §5).
- Setup never advances after a failed validation; secure-cookie handling for HTTP bootstrap; HTTPS proof not based on forwarded headers (02 §1).
- One lock for every apply path; applied plan equals previewed plan (01 §5).
- Values never string-interpolated into config; IPv6 brackets; invalid resource can't break rendering (04 §1.5).
- Rollups only from closed buckets; history strips keep timestamps (01 §4.3, 07 §4).
- Scheduled scripts get env/timeout/redaction/history; all-disabled targets = no hosts (09 §1).

Many other V2 findings came from V2's own code structure (EF migrations, .NET DI wiring, Postgres volumes) and cannot
occur in the new stack, so they are not in the spec.

## 6. Open questions

None.
