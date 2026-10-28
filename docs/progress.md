# Progress

The single place that says where the project stands. Updated at the end of every working session.
Rules: docs/spec/11-delivery.md §2.

## Current slice

**S2 — DNS read + import** · status: **in progress** (planning, 2026-10-28).

Scope and gate: 11-delivery.md §3 row S2 — 03-dns.md §1–4 (read side), setup step 3 (02 §1.3). Gate: the real zone is
listed; the import table shows the real RRSets; import decisions persist; sanitized fixtures committed.

External systems: Hetzner Cloud API (DNS zones and RRSets). No contract yet → captured first (read-only).

Contract
- [ ] `make contract-capture PROVIDER=hetzner`: list zones (paginated), get zone, list RRSets (paginated), bad token
      (401), unknown zone (404); raw into `.contracts-raw/`, sanitized into `testdata/contracts/hetzner/` with
      `captured_at` + docs URL; denylist check. Needs a Cloud project token (user).

PR A — backend
- [ ] Migration 00003: DNS provider connection (type, default TTL, token as vault secret), `dns_zones`,
      `dns_observed_rrsets`, `dns_ownership`, `dns_import_decisions`; sqlc queries.
- [ ] `internal/dns`: provider interface (read part: Capabilities, ListZones, ResolveZone, ListRRSets), RRSet model and
      normalization (TXT, TTL default); `internal/dns/hetzner` read adapter from the fixtures (pagination, RateLimit
      backoff, legacy-token error); replay tests; `TestInvariant_2_*` (NS/SOA never selectable for prune) on the import side.
- [ ] Zone refresh job (observed snapshot) + OpenAPI: `GET /api/dns/zones`, `POST …/refresh`, `GET …/rrsets`,
      `GET /api/dns/import/{zoneId}`, `PUT …/decisions`; setup step 3 endpoints; `make generate`.

PR B — UI + deploy
- [ ] Setup step 3 (provider, token, zone auto-pick, read-only validation) and the import table.
- [ ] `/dns`: Records (observed, origin, read-only in S2) and Import tab with decisions, from the approved mock.
- [ ] Deploy (user OK) and prod-proof: real zone listed, import table matches the zone, decisions survive a restart.

## Side step: security model (branch `security-model`)

Status: **done** (signed off 2026-10-28, mocks merged). S2 is next.

The user replaced the security design with a model built on two shared primitives: IP sets (the one address primitive,
also for DNS values, upstreams and connection targets, so anything can be "set via Pulse") and rule sets (global,
reusable and service-root rule sets are one type), a two-stage edge engine (Stage A abuse engine before Stage B rules),
one ban function, and nftables with folded allow/block sets and kernel meters. Original input archived in
`docs/archive/` (not for implementation); decisions, corrections and rejected alternatives in ADR 0002.

Done on the branch:
- [x] ADR 0002; spec rewritten: 05-firewall, new 05a-ip-sets, 06-edge-security, 06a (object model §0: what is shared,
      what is compiled per layer, deliberate deviations); updated 01, 02, 03, 04 (renamed `04-traefik-services.md`),
      07, 08, 09, 10, 10b, 11, README (invariants 7, 10, 12 amended; 20–22 added), environment, CLAUDE.md.
- [x] Read-only survey of the first firewall host: kernel 7.0, nft 1.1.3, iptables **legacy**; NetBird uses legacy
      chains + ipsets; old script chains still present; Proxmox firewall disabled; no nftables tables;
      `nftables.service` disabled with the stock `flush ruleset` config.
- [x] Counters view (ban + challenge counter, filters, edit/reset).
- [x] Review round 2026-10-28: step decay for both counters; an SSO session satisfies `pass_captcha`; observe/detect
      modes stay out; host link stays SSH (ADR 0002 "Host link vs host agent"); Pulse footprint targets (08 §1).

After sign-off (phase 2):
- [x] Merge the spec branch (docs → `main`).
- [x] Redo the affected S0b mocks (Services, Security incl. Counters, address page, IP sets, rule sets, Firewall) —
      branch `s0b-security-mocks`.
- [x] Redone mocks approved by the user (2026-10-28), plus host and per-service routing (03-dns.md §5.1 `ovr` records).
- [x] Mocks PR juzo/hashi#27 merged.
- [x] Built code checked: only setup step 7's key/title and the palette mock said "resource"/subjects; renamed.
- [ ] Then continue with S2.

## Finished slices (detail)

**S0b — UI design for all features** · done, approved by the user 2026-10-27 (not deployed; the user reviewed screenshots
and the local preview). `docs/spec/10b-ui-system.md` + every page as a mock under `/design` (#24), four themes, contrast
tests, phone layout. Feature slices build the real page at the same path without `/design`.

**S0 — Walking skeleton** · done 2026-10-27, deployed `e7c0a30` to the Docker LXC (`hashi-v3`, ports 3340–3342).
PRs #17–#23: Go module + tools, `make check`, CI, redacting logs and `secrets.Value`, SQLite + goose migrations with
pre-migration backup, job runner, three listeners + `/healthz` `/readyz`, OpenAPI 3.1 → oapi-codegen strict server + TS
client, embedded SvelteKit SPA, multi-arch distroless image, release workflow, `make deploy/rollback/smoke`. Review fixed a
rollback bookkeeping bug and added missing invariant 6 / readyz tests.

Decisions made in S0:
- SvelteKit **2** kept as specified; Kit 3.0 (2026-10-01) skipped for now — the component ecosystem still targets Kit 2.
- shadcn-svelte init moved to S0b, where the design tokens are defined.
- Deploy targets live in gitignored `deploy/targets/<env>.env` (example in `deploy/targets.example/`).
- `make dev`, `make image`, `make deploy` exist now; `make test-integration`, `make test-e2e`, `make contract-capture` arrive with the slices that need them.

## Slices

| Slice | Status | Prod-proof (date · version · evidence) |
| --- | --- | --- |
| S0 Walking skeleton | **done** | 2026-10-27 · `e7c0a30` · `make deploy ENV=prod TAG=e7c0a30` on the Docker LXC next to V1: smoke from the LAN → `/readyz` `{"checks":{"database":"ok"},"status":"ok"}`, `/api/version` `{"commit":"e7c0a30","version":"e7c0a30"}`, SPA 200, public apps 200, public status 200; container `healthy`, uid 65532, read-only rootfs; restart keeps the DB (no re-migration); V1 and Gatus still up. CI green on the Gitea runners (#17–#23 and main); release published `e7c0a30` = `latest`, amd64 + arm64. |
| S0b UI design for all features | **done** | 2026-10-27 · `646888e` · approved by the user from screenshots and the local preview; not deployed (not part of this gate) · #24 |
| S1 Setup, auth, vault | **done** | 2026-10-27 · `8e5c1b5` · `make deploy ENV=prod TAG=8e5c1b5`: readyz ok, version `8e5c1b5`, SPA/public 200. Bootstrap login from the PVE host (LAN) → 200, `hashi.bootstrap` cookie (HttpOnly, SameSite=Strict, no Secure over HTTP), session bound to `10.0.4.1`, scopes `setup.manage` only. `docker restart hashi-v3` → same cookie still authenticated, bootstrap block not printed again. Cookie replayed from inside the Docker LXC (`10.0.4.106`) → `authenticated:false` + cookie cleared; afterwards rejected from `10.0.4.1` too; audit `session.revoked` `{bound_ip:10.0.4.1, reason:ip_mismatch}` observed ip `10.0.4.106`. DB + WAL copy: bootstrap password, master key (base64 and raw) absent; `secret_records` holds only AES-GCM ciphertext; session ids stored as 32-byte sha256; bootstrap hash argon2id. Passkeys: CI E2E with virtual authenticator green on `8e5c1b5` (no HTTPS before S5). Note: from the laptop, 10.0.4.0/24 is reached through kanae (NetBird route, SNAT), so the laptop appears as `10.0.4.1`. |
| S2 DNS read + import | next | |
| S3 DNS write | — | |
| S4 SSH + Traefik | — | |
| S5 HTTP resources | — | |
| S6 Firewall + streams | — | |
| S7 Monitoring + public pages | — | |
| V1 cutover | — | |
| S8 AdGuard, Pulse, agent DNS | — | |
| S9 Edge SSO + rules | — | |
| S10 Security engine, WAF, CAPTCHA, blocklists | — | |
| S11 Scripts, themes, settings | — | |
| S12 Hardening + 1.0 | — | |

## Blocked

(Problems that survived two fix attempts: hypothesis, evidence, question for the human.)

## Unverified

(Review findings that were not reproduced. Not worked on unless promoted.)

- S1 review (#26): Activity → Live log keeps its last sequence number across a server restart (sequence restarts at 0),
  so an open tab could skip the startup lines after a restart. Read from code, not observed.
- S0 review: for a `v1.2.3` git tag the image tag is `1.2.3` (metadata-action `{{version}}`) but the binary reports
  `v1.2.3`, so `make deploy TAG=1.2.3` would fail its smoke version match (`deploy/deploy.sh` smoke). Not observed;
  only short-SHA deploys exist so far.

## Log

- 2026-10-28 — Security model side step done: mocks merged (#27), built code renamed (setup step 7), S2 next.

- 2026-10-28 — Security model signed off by the user (ADR 0002 accepted). Branch merged to `main`; mocks next.

- 2026-10-28 — Security model review round with the user: step decay, SSO satisfies CAPTCHA, no detect mode, SSH
  host link confirmed in the ADR, Pulse footprint targets. Still awaiting final sign-off.

- 2026-10-27 — Security model side step: the user's new model turned into spec on branch `security-model` (ADR 0002,
  05a new, 05/06/06a rewritten, Resource → Service). First firewall host survey: everything on iptables-legacy, NetBird too → S6 starts
  with an nft/NetBird coexistence gate, iptables+ipset as plan B. Awaiting sign-off.

- 2026-10-27 — S1 signed off by the user. README bar 3/15. S2 not started on the user's request.

- 2026-10-27 — Deployed `8e5c1b5` (S1) to the Docker LXC after the user created `master.key` on the host; smoke ok;
  S1 prod-proof checks passed (see the S1 row). Old S0 image can't run on the migrated DB (pre-migration backup kept).

- 2026-10-27 — S1 code: #25 (backend) merged, #26 (UI, E2E, deploy) open. Reviews reproduced and fixed: short idle
  limits expired active sessions; changing the root domain during setup stranded passkeys; admin pages stayed on a dead
  error after their session ended. The secret guard caught a real inventory IP used as a test fixture before any push;
  history was rewritten locally (nothing published). Lesson: fixtures use RFC 5737 / neutral private ranges only.

- 2026-10-27 — S0b approved by the user. Next: S1.
- 2026-10-27 — S0b mocks merged (#24): `docs/spec/10b-ui-system.md` + every page under `/design`. One PR of ~5,500
  hand-written lines (mostly page mocks reviewed by clicking), kept whole on purpose. Found: the shadcn-svelte nova
  components need the CLI's `tailwind.css` variants (`data-open` → `data-state="open"` …) that `init` normally writes;
  vendored as `web/src/lib/styles/shadcn-svelte.css`.
- 2026-10-27 — S0 gate passed (`e7c0a30` on the Docker LXC). PR sizing rule changed from ~500 lines to per-slice
  reviewable PRs (spec 11-delivery §2.7). V1 image workflow restored for `legacy/v1` (V1 code changes only); CI and
  release skip docs/V1-only changes. README progress bar added. Repo moved out of iCloud-synced `~/Documents`.
- 2026-10-27 — S0 code done (PRs #17–#23). Found along the way: `.gitea/workflows/v1-docker-build.yml` on main has
  only a trigger, no jobs (cut off in the restart commit), so V1 images can't be rebuilt — left alone because a rebuild
  would push `:old`, which production runs. The repo sits in iCloud-synced `~/Documents`; iCloud once renamed a
  build dir to `dist 2` mid-build, which got committed by mistake and reverted.
- 2026-10-27 — Restart. V2 attempt removed from history (backup bundle kept off-repo). New spec in `docs/spec/`.
