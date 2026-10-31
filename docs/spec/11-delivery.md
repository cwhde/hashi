# 11 — Delivery: slices, prod-proof gates, testing, CI/CD, cutover, operations

## 1. Why this section exists

V2 produced 450 commits in three weeks and was never deployed. Its first contact with reality (an audit, months later)
found that the DNS adapter targeted a shut-down API, Traefik validation called a command that does not exist, firewall
NAT chains were in the wrong table and setup could not complete. Eight AI audit series then produced 353 issue files;
fixing findings created new findings. These rules exist so that cannot happen again. They are enforced partly by
process (below) and partly by tooling (`.claude/` hooks, CI).

Known pitfalls worth remembering (the rest of V2's findings don't apply to a fresh build):

- Hetzner DNS is only the Cloud API now; the legacy DNS API is gone.
- Traefik has no config-check command; verify through its own API after loading.
- NAT rules belong in a `nat`-type chain, and DNAT targets the Traefik entry-point port.
- nftables: conntrack (`ct state`, `ct count`) is only available after priority −200, so meters cannot run at `raw`;
  Debian's `nftables.service` starts with `flush ruleset` and must never be enabled by Hashi.
- A firewall rescue timer is disarmed only after a *new* connection succeeds.
- Config and scripts are rendered from structs, never string-templated from user input.

## 2. Delivery rules

1. **One slice at a time (WIP = 1).** A slice is a vertical piece of user-visible behaviour, built end-to-end (DB → API →
   UI → real system). The next slice starts only after the current one has passed its prod-proof gate.
2. **Prod-proof gate.** A slice is done only when its behaviour has been observed working against the **real** systems it
   touches, on the real deployment, and the evidence is recorded in `docs/progress.md` (date, version, what was done,
   what was observed — command output or screenshot references). Tests passing is necessary, not sufficient.
3. **No invented interfaces.** Every external API, CLI or file format Hashi talks to is captured from the real system
   first (`make contract-capture`), stored sanitized in `testdata/contracts/`, and tests replay it. If something cannot be
   captured, it is listed as an open question — the agent stops and asks; it does not guess.
4. **Deploy early.** Slice S0 ends with a running container on the real Docker host. Every later slice is deployed there
   before it is called done.
5. **Fix discipline.** A bug fix starts with a reproduction: a failing test or a recorded failing observation on the real
   system. If two fix attempts for the same failure do not make the reproduction pass, stop, write the hypothesis and
   evidence into `docs/progress.md` under "Blocked", and ask the human.
6. **Review discipline.** One review per change, against the spec section's acceptance criteria, maximum 10 findings,
   style skipped. A finding is only "must fix" when it is reproduced or violates an invariant. Unreproduced findings go to
   `docs/progress.md` → "Unverified" and are not worked on unless the human promotes them. No standalone audit series.
7. **Reviewable changes, not tiny ones.** One PR per coherent piece of a slice — usually one or two per slice, split
   along natural seams (backend, UI, deploy) only when a single PR would be too big to read in one sitting (rough guide:
   ~1,500 hand-written lines, excluding generated code, lockfiles and fixtures). No stacked micro-PRs: every PR costs a
   full CI run. Docs and small fixes go straight to `main`.
8. **Spec changes are explicit.** If implementation reveals the spec is wrong, update the spec in a separate commit
   (`docs(spec): …`) with the reason; never silently diverge.
9. **The repository is public.** Real hostnames, IPs, tokens and production configs live outside the repo
   (`CLAUDE.local.md`, `.env`, deployment host). Fixtures are sanitized before commit (see §5.2).

## 3. Slices

Each slice lists its scope (spec references) and its prod-proof gate. All slices are required for 1.0; test deploys
happen at the end of every slice.

| # | Slice | Scope | Prod-proof gate |
| --- | --- | --- | --- |
| S0 | Walking skeleton | Go binary, embedded SPA shell, `/healthz` `/readyz`, SQLite + migrations, job runner skeleton, OpenAPI pipeline, CI (lint/test/generate check), multi-arch image, `make deploy` | Container runs on the Docker LXC next to V1; `/readyz` OK from LAN; CI green on Gitea runners; image published |
| S0b | UI design for all features | 10-ui.md, 10a, every area file's UI parts | Before any feature page is built: page inventory for every feature in the spec, navigation grouping, layout and component rules, design tokens for the four themes, and every page built as a clickable mock inside the SPA (`/design` route, mock data, all four themes). Approved by the user; later slices fill these pages with real data |
| S1 | Setup, auth, vault | 02-setup-auth.md §1.1–1.2, §2–5 (service mode), audit log, settings framework, Activity page | Bootstrap login from LAN, session IP binding observed, restart keeps sessions, secrets encrypted in a DB dump; passkey registration and login pass with a virtual authenticator in tests (no HTTPS before S5, see 02 §1) |
| S2 | DNS read + import | 03-dns.md §1–4 (read side), contract capture for Hetzner | Real zone listed; import table shows real RRSets; decisions persist; fixtures committed (sanitized) |
| S3 | DNS write | 03-dns.md §5–7, sync engine 01-platform.md §5, approval UI | Probe TXT created + deleted via approved plan on the deployed install (setup step 3's write test: admin sessions need a passkey, which needs S5's HTTPS); a Hashi-owned test record applied, verified and its delete approved, by the same build against the real zone with a virtual authenticator (as S1); NS/SOA untouched. S6 repeats the record path on the deployed install with the first real passkey (registered in S5) |
| S4 | SSH + Traefik | 04-traefik-services.md §2–6, sshx with host-key pinning, Hashi-issued certificates (03 §5.3) | Hashi-installed Traefik running on a fresh edge LXC; rendered files applied; Verify reads routers from the real Traefik API; Hashi issues the root + wildcard certificate via DNS-01, distributes it, and the edge serves it (fingerprint checked); forward-auth fallback observed with Hashi stopped, using a stub forward-auth endpoint (06 §2.1) |
| S5 | HTTP services | 04 §1, §7–8 (HTTP kinds; targets are single addresses until S6 brings address sets), system service (setup steps 7–8), deeper wildcards (03 §5.3) | Hashi admin reachable at its HTTPS domain through Hashi-managed Traefik (setup step 8 proof on the real install); the first real passkey registered from a real browser on a temporary admin domain (V1 keeps its domain until cutover); one real service served by the new Traefik and reachable through it. The temporary admin name resolves through a manual DNS entry (generated service records need firewall hosts, S6) |
| S6 | IP sets + firewall + streams | 05a-ip-sets.md (`ip`, `cidr`, `host`, `dns`, set references, system sets, compiler, IP sets pages, address page without dynamic state), 05-firewall.md (nftables table, folding, bindings, host access, rescue; meters rendered but off), TCP/UDP services, DNS host and service records (03 §5.1–5.2), setup step 6; **entry gate first**: nft/NetBird coexistence capture (05 §10) | First apply with rescue timer on a real host; foreign + NetBird rules unchanged (diff of `nft list ruleset` minus Hashi's table and of `iptables-legacy-save` before/after); NetBird connected after apply and after a NetBird restart; an address in a block set is dropped, an allowed /32 inside it passes (observed with real test sources); public TCP/UDP port reaches its upstream; Hashi's table survives `hashi-firewall` unit restart; a DNS record whose value is an address set applied; setup completes end-to-end on the real install (step 6 is the last missing step), sign-in with the passkey registered in S5, and the S3 record path repeated on the deployed install with it; a migrated service's generated CNAME applied |
| S7 | Monitoring + public pages | 07-monitoring-public.md | All migrated services monitored; public status + dashboard served on their ports; a real outage produces a notification and recovery |
| S8 | AdGuard, Pulse, agent DNS | 03 §8–9, 08-pulse.md, `pulse` IP items | Real AdGuard rewrites synced without touching manual ones; Pulse agent on a real host; an address change propagates through its IP sets to a DNS record, a service upstream and the folded firewall `allow` without a manual step; `<agent>.hashi.home.arpa` resolves |
| S9 | Forward auth, rule sets, edge SSO | 06 §2, §4, §7, §12, §13 (rule set pages, service Access tab), `geo` items (L7 and firewall expansion) | SSO login covers two subdomains; a shared rule set attached to two services decides on both (trace observed); rules by IP set, by Pulse-backed set and by country observed; `deny` fallback service observed; fail-closed vs fail-open by rule content observed with Hashi stopped; forward-auth latency from the remote (home) Traefik host recorded |
| — | **V1 cutover** | §7 | Every service moved to Hashi's edge; each app Pangolin protected with SSO asks for a login from a signed-out browser on its public name before and after the move; V1, Gatus and Pangolin stopped; Hashi owns DNS/monitoring for everything V1 handled |
| S10 | Abuse engine, WAF, CAPTCHA, feeds | 06 §3, §5–6, §8–11, §13 (Security dashboard, Engine), dynamic sets, host link, kernel meters on | Access log tailed live from real Traefik; synthetic L7 load → challenge → Cap solve → clear; ignored challenge → ban in `dyn` on every managed host within one poll interval; a test source tripping an L4 meter is staged, drained and banned with `T_ban(c)`; an allowed address tripping both is never banned; a feed fetched and enforced via the firewall block binding; WAF match counted; `headers_only` path accepts a large upload |
| S11 | Scripts, custom themes, settings completeness | 09-scripts-settings.md, 10-ui.md §2 | Scheduled script runs on real hosts with history; custom theme editor works; visual tests pass in all four themes; every setting has a consumer test |
| S12 | Hardening + 1.0 | strict vault mode (02 §5.4), backup/restore drill, threat model, docs | Restore from backup onto a fresh container works; strict mode unlock with PRF passkey or recovery key; security scans clean or accepted; tag `v1.0.0` |

A slice may span several PRs (rule 7); the gate stays at the slice level.

## 4. Testing

| Layer | What | Where |
| --- | --- | --- |
| Unit | pure logic: planners, renderers (golden files), decision engine, normalizers, policies | `go test ./...`, Vitest |
| Contract | replay recorded real responses through adapters; drift job re-captures read-only shapes weekly | `make test-contract` |
| Integration | real binaries in containers: Traefik (pinned), sshd, iptables/ipset in a privileged netns, AdGuard Home, OIDC test provider, SMTP sink | `make test-integration` (CI job with Docker) |
| E2E | Playwright against a running Hashi with fake/contract-backed providers; WebAuthn via virtual authenticator | `make test-e2e` |
| Visual | Playwright screenshots of key pages per theme and width (10-ui.md §4) | `make test-visual` |
| Prod-proof | manual/semi-automated checklist on the real deployment | `docs/progress.md` |

Invariant tests (`TestInvariant_*`) run in every CI run. Safety tests from V2 are kept: NS/SOA deletion impossible;
unowned records untouched; identical Traefik file not rewritten; firewall does not flush unrelated rules (any backend); NetBird rules
preserved and access survives apply; manual allow beats dynamic bans; unresolved targets keep applied state; Pulse IPs matching hosts produce CNAMEs; secrets redacted in logs and API responses;
passive sync runs without a session; high-risk passive plans wait for approval.

`make check` = format check + lint + unit + contract + generate-diff + web check/lint/unit. It must run in < 3 minutes
locally; it is what the agent's Stop hook runs.

## 5. Contract capture

### 5.1 Procedure

`make contract-capture PROVIDER=<name>` runs read-only calls from `internal/<area>/contract/capture.go` using credentials
from the local environment (never from the repo), writes raw responses to `.contracts-raw/` (gitignored), then sanitizes
into `testdata/contracts/<provider>/<call>.json` with a header (`captured_at`, `docs_url`, `tool_version`).

### 5.2 Sanitizing

Tokens, IDs that embed account data, IPs and domains are replaced deterministically (`example.com`, `192.0.2.x`,
`198.51.100.x`, `2001:db8::/32`). A test fails if a fixture contains anything matching the private-data patterns in
`testdata/contracts/denylist.txt` (populated locally from `CLAUDE.local.md`, the list itself is gitignored; CI uses a generic pattern set).

## 6. CI/CD (Gitea Actions, self-hosted runners)

### 6.1 Workflows

- `ci.yml` — on PR and push: path-filtered jobs; `make check`; integration tests (Docker); E2E only when `web/` or API changed; Pulse vet/test/build.
- `security.yml` — on PR, push to main, weekly schedule, manual: gitleaks, govulncheck, osv-scanner (Go + pnpm lockfiles), Semgrep, Trivy filesystem; Trivy image scan of the **pushed digest** for main/tags (local single-platform build only for PRs). Docs-only changes skip heavy scans unless scheduled/dispatched.
- `release.yml` — on main and `v*` tags: multi-arch images `git.juzo.io/juzo/hashi` and `git.juzo.io/juzo/hashi-pulse` (linux/amd64 + linux/arm64), tags `latest` (main), short SHA, semver; Pulse binaries + SHA-256 checksums attached to tag releases.
- `v1-docker-build.yml` — legacy V1 image (paths `legacy/v1/**`), kept multi-arch until cutover, then removed with `legacy/`.

Versions: `v0.<step>.<patch>` until `v1.0.0` (S12). The middle number is the slice that passed its gate (`v0.8.0` when S8
passes), the last counts fixes after it. While a slice's gate is open, `v0.8.0-beta.1`, `-beta.2`, … are pre-releases. The
V1 cutover is released on the slice it follows. Every other image stays tagged by its short commit.

### 6.2 Build performance (from V2 addendum, still required)

- Dockerfiles use BuildKit syntax with cache mounts; build stages pinned to `$BUILDPLATFORM`; Go cross-compiles with
  `GOOS=$TARGETOS GOARCH=$TARGETARCH CGO_ENABLED=0` (pure-Go SQLite makes this trivial) — no QEMU-emulated toolchains.
- Node SPA build runs once on `$BUILDPLATFORM` (architecture-independent output).
- Caches: Go build + module cache, pnpm store, Playwright browsers (only installed when E2E runs). Registry build cache on.
- Cache misses never fail builds; logs state hit/miss.
- Scan the release digest rather than rebuilding for scanning.
- A CI check asserts Dockerfiles use `$BUILDPLATFORM`/`TARGETARCH` and workflows keep both platforms.

### 6.3 Deploy

`make deploy ENV=prod` (human-triggered skill `/deploy`): SSH to the Docker host from `CLAUDE.local.md`, `docker compose pull`,
`up -d`, wait for `/readyz`, run smoke checks, record version in `docs/progress.md`. `make rollback` returns to the
previous image tag. Compose file and `.env.example` in `deploy/compose/`; the real `.env` and master key live only on the host.

## 7. V1 cutover

Today (environment.md): V1 writes `<sub> → on.<host>` CNAMEs from a topology TXT record and Pangolin's resource list and
generates Gatus endpoints; Pangolin feeds both Traefik instances through its HTTP provider; a cron script runs host A's firewall.
Hashi takes over in this order, each step reversible:

1. **S2–S3 (DNS):** import every RRSet V1 created as `imported`; Hashi's DNS plan for that set must show **no changes**
   while V1 still runs (proves equivalence). V1 keeps writing until step 5.
2. **S4–S5 (Traefik):** the current reverse proxies are **replaced, not adopted**. Hashi installs its own Traefik
   (04-traefik-services.md §2–3, Hashi static config, Hashi paths, certificates issued by Hashi) on a fresh edge LXC per
   host; nothing of the Pangolin-generated or hand-written Traefik config is reused. Every Pangolin resource is recreated as a
   Hashi service; once the new instance serves them all (verified via its API and real requests), the firewall DNAT target /
   router forward moves to the new LXC. That move happens at the cutover (step 5), after S9: until then forward auth has no SSO, and no app that Pangolin protects may lose its login, even briefly. Pangolin keeps serving all public traffic until then. No temporary allow-list replaces SSO.
3. **S6 (firewall):** Hashi's first firewall apply on host A removes the old script's chains, jumps and cron entries under the
   rescue timer. The topology TXT record is no longer needed afterwards.
4. **S8 (AdGuard):** Hashi writes rewrites to every AdGuard instance directly; adguardhome-sync is then retired.
5. **After S9 → cutover:** monitoring covers all former Gatus endpoints, and each app Pangolin protects has a Hashi rule set requiring SSO, tested on the test edge first. Move traffic to the new edge; a signed-out browser on each protected public name must get the login. Then stop V1, Gatus, Pangolin (+ its Traefik) and the old
   Traefik LXC; keep their data for one week as rollback; then delete them, `legacy/v1/` and its workflow.

## 8. Operations

- **Backups (mandatory):** nightly `VACUUM INTO /data/backups/hashi-<date>.db` (online, consistent), 14 kept; `/data`
  (DB backups, GeoIP, assets, uploads) copied off-host by the host's backup system (Proxmox backup of the LXC is fine).
  Master key and recovery key are stored outside Hashi (password manager).
- **Disaster recovery:** restore `/data` → start Hashi with the master key (or recover it with the recovery key) → unlock
  (strict mode) → run global reconcile → review plan before approving any high-risk item.
- **When locked:** see 02-setup-auth.md §5.5.
- **Upgrades:** migrations run at start inside a transaction after an automatic pre-migration backup; failure restores
  the backup and exits non-zero.
- **Observability:** JSON logs to stdout; `/metrics` (Prometheus) on the admin listener behind admin auth or a separate
  bind address.
