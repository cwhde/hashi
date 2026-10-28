# Progress

The single place that says where the project stands. Updated at the end of every working session.
Rules: docs/spec/11-delivery.md §2.

## Current slice

**S4 — SSH + Traefik** · status: **in progress** (started 2026-10-28)

Scope and gate: 11-delivery.md §3 row S4: 04-traefik-services.md §2–6, sshx with host-key pinning, Hashi-issued
certificates (03 §5.3), setup steps 4 (certificate provider) and 5 (Traefik connection). Gate: Hashi-installed Traefik
running on a fresh edge LXC; rendered files applied; Verify reads routers from the real Traefik API; Hashi issues the root +
wildcard certificate via DNS-01, distributes it, and the edge serves it (fingerprint checked); forward-auth fallback
observed with Hashi stopped, using a stub forward-auth endpoint (06 §2.1).

Dev targets: three temporary LXCs on host B, one per OS family Hashi installs on: `hashi-edge-test` (Alpine 3.23,
OpenRC, sshd without forwarding), `hashi-edge-debian` (Debian 13, systemd) and `hashi-edge-fedora` (Fedora 44, systemd,
dnf). Same size and settings as the first (see Log 2026-10-28); not the production edge.

External systems and contracts:
- Traefik v3.7.16 release, static/dynamic config, `/api/rawdata`, cert reload, forward-auth fallback: captured
  2026-10-28 (`testdata/contracts/traefik/`). Spec corrected from it (3e0bef4).
- OS probe over SSH (Alpine 3.23, Debian 12): captured (`testdata/contracts/os/`).
- sshd: real `sshd` in the integration container (x/crypto/ssh + pkg/sftp, 01 §2), no fixture needed.
- Hetzner `add_records`/`remove_records` for `_acme-challenge`: already captured (`testdata/contracts/hetzner/write/`).
- ACME: Pebble (the ACME test server, with EAB required) in the integration container; Google Trust Services in the gate.
  User decisions 2026-10-28: CA = **Google Trust Services** (EAB; the user creates a fresh EAB key for the gate, the one
  in the old Traefik config is not reused); Hashi may write and remove `_acme-challenge` TXT on the real zone, in
  development and on the server (nothing else beyond `_hashi-test`).

Scope cut: services (S5) are not rendered yet, so `10-`/`20-hashi-*` files stay absent; the gate's route is a Hashi health
router on the edge. Deeper wildcards come with S5 services. The Traefik install is proven on all three dev targets
(Alpine/OpenRC, Debian/systemd, Fedora/systemd + dnf); Ubuntu shares Debian's path. Forward auth is a stub that allows everything (the engine is S9/S10).

PR A — backend
- [x] `internal/sshx`: x/crypto/ssh client, mandatory host-key pin (scan → user confirms the SHA256 fingerprint → stored),
      key or password auth, Hashi's own ed25519 key (public half as an authorized_keys line), `Run` with capped output,
      SFTP read/hash, atomic write (same-dir temp + chmod + posix-rename, skipped when the hash matches), `Fetch` of
      loopback URLs through the SSH connection or the host's curl/wget. `make test-integration` + CI job with a real
      OpenSSH container (Alpine 3.23): pin, both logins, changed host key refused, atomic writes, both fetch paths.
- [ ] Migration 00005: certificate provider (ACME email, EAB, account key ref), `traefik_connections` (target, port, user,
      credential refs, pinned host key, internal IP, OS, paths, state), `traefik_files` (per-host `last_applied_hash`),
      `certificates` (+ per-host distribution state); sync subsystems `traefik` and `certificates` in the CHECK + OpenAPI enum.
- [ ] `internal/traefik`: OS probe parser (fixtures), install items (Hashi downloads the pinned release, checks the
      embedded SHA-256, uploads the binary over SFTP; directories; OpenRC script / systemd unit; existing-config discovery +
      backup + explicit ownership), static config and dynamic files as Go structs → YAML (`00-hashi-core`,
      `40-hashi-security` with the captured fail-open/closed construct, `90-hashi-health`, `hashi-certs` with hashes).
- [ ] Apply + Verify (04 §6): host hashes over SFTP, atomic writes, static change → restart + wait for ping + restore on
      failure, Verify = `/api/rawdata` through the SSH tunnel (every expected router/service/middleware present, none
      disabled), `last_applied_hash`. Golden render tests, JSON-schema check, `TestInvariant_8_*` (identical render → no write),
      integration: real Traefik v3.7.16 container loads the files, API reports no errors, request reaches a stub upstream.
- [ ] Certificates: lego with a DNS-01 provider on Hashi's DNS adapter (`add_records`/`remove_records`, origin `system`,
      excluded from import/prune), keys in the vault, root + `*.root`, job `certificates.renew` (daily, 30 d renew,
      warnings at 21/7 d), distribution in the `traefik` plan, Verify = TLS handshake + fingerprint. Integration with Pebble.
- [ ] Stub `GET /api/edge-auth/forward` (204) on the admin listener; health router on the edge uses it fail-open.
- [ ] OpenAPI + handlers: setup steps 4 and 5 (`/api/setup/certificates`, `/api/setup/traefik/*`: scan host key, confirm,
      validate, install plan), `/api/traefik/connections/*`, `POST /api/traefik/plan`, `GET /api/traefik/certificates`,
      `POST /api/traefik/certificates/renew`. `TestInvariant_1/5/7/13_*` for the new adapters.

PR B — UI + deploy
- [ ] Setup steps 4 and 5 (host-key fingerprint confirmation, Hashi's public key to copy, validation results, install plan).
- [ ] `/traefik` page from the mock: connection state, files with hashes, Certificates tab, "Renew now"; plans on `/changes`.
- [ ] E2E against fakes (dev SSH/Traefik fake host, Pebble-less fake ACME) for setup steps 4–5 and the install plan.
- [ ] Deploy (user runs `/deploy`) and prod-proof on `hashi-edge-test` (full gate) plus the install plan, files and Verify
      on `hashi-edge-debian` and `hashi-edge-fedora`: host key pinned, install plan applied (Traefik under OpenRC/systemd), files applied, Verify from the real API, real certificate issued + distributed + fingerprint matches, Hashi
      stopped → fail-open route 200 / fail-closed 500 on the edge, Hashi back → normal; live Traefik instances untouched.

## Previous slice

**S3 — DNS write** · status: **done** 2026-10-28, deployed `94d60ad` (#30, #31), prod-proof passed.

Scope and gate: 11-delivery.md §3 row S3: 03-dns.md §5–7, sync engine 01-platform.md §5, approval UI. Gate: a probe
TXT is created and deleted through an approved plan; a Hashi-owned test record is applied and verified; NS/SOA are untouched.

External systems: Hetzner Cloud API writes. The contract is already captured (`testdata/contracts/hetzner/write/`,
2026-10-28), so nothing new to capture.

Scope cut: §5 host and service records need firewall hosts (S6) and services (S5), and A/AAAA address-set values need
IP sets (05a). Those arrive with their slices on top of this engine. S3 delivers manual entries with literal values,
imports, prunes and the probe, all through plan → approve → apply → verify.

PR A — backend
- [x] Migration 00004: `dns_records` (desired manual entries: name, type, TTL or zone default, values, display name,
      dashboard/monitoring flags), `sync_plans`, `sync_plan_items`, `sync_runs`, `sync_locks`, observed `owner`; sqlc queries.
- [x] Hetzner write adapter from the write fixtures: create, `set_records`, `change_ttl`, delete, owner label (`PUT`),
      Action polling with timeout, TXT quoting/escaping/255-byte split; replay tests for every write fixture used.
- [x] `internal/dns` planner: desired (owned rows) vs observed → create/update/label/delete/conflict items; NS/SOA and
      unowned RRSets never planned (conflict instead); CNAME exclusivity and apex checks; deletes are high risk.
- [x] `internal/sync` engine: plan + items + hash, low-risk auto-approve, high-risk approval, apply under the lock with
      re-plan (a new high-risk item goes back to approval), verify by read-back, `last_applied_hash`, run row, audit.
      Background reconcile in the `dns.refresh` job.
- [x] Probe plan (create `_hashi-test` TXT → verify → delete → verify, high risk); import → desired entry `imported`
      (+ owner label as a low-risk item); prune → one high-risk plan listing every RRSet.
- [x] OpenAPI + handlers: `POST /api/dns/records`, `PATCH|DELETE /api/dns/records/{id}`, `POST /api/dns/plan|prune|probe`,
      `GET /api/sync/plans`, `GET /api/sync/plans/{id}`, `POST /api/sync/plans/{id}/approve|apply|discard`,
      `POST /api/setup/dns/probe`, `POST /api/setup/dns/probe/{id}/run`. Tests `TestInvariant_1/2/3/4/7_*` for the write side.

Decisions made in PR A:
- A user entry counts as Hashi's at the provider only after Hashi created it (`last_applied_hash` set). An RRSet that
  appears in the console between saving and planning becomes a conflict item, never an overwrite (invariant 1).
- Conflict-only drafts create no plan (the background reconcile would otherwise make one every hour); saving an entry
  over an unowned RRSet is refused up front with "import it instead".
- Owner marker = label `hashi.io/owner=<instance id>` (new setting `dns.instance_id`, a lowercase ULID); imported
  RRSets get it as a low-risk "label" item, other labels are kept.
- Updates use `set_records` and `change_ttl` only (`add_records`/`remove_records` are captured but not needed).
- Low-risk plans apply in the saving request (no 2 s debounce yet; one save = one plan). `GET /api/sync/runs` dropped:
  recent plans carry the result. The probe in setup step 3 is approved by the click on the shown plan (there is no
  passkey yet in setup); after setup it needs `sync.apply` + reauth like every high-risk plan.
- `make dev`/E2E use a stateful Hetzner fake (`hetznertest.Sim`) seeded from the recorded zone, so plans apply locally.
- PR A is ~4,000 hand-written lines (about half tests): one coherent backend, kept whole.
- Review #30 (all four reproduced, regression tests added): S2-era imports get their entry before planning (else
  they became deletes); entry deletes are their own subsystem `dns-record-deletes` so a waiting delete never holds
  back safe changes (invariant 16); after a failed apply/read-back the zone is re-read and what took effect stays
  Hashi's; the write test deletes its probe even after a failed create. Prod has no S2 imports (all 58 were "leave").

PR B — UI + deploy
- [x] `/changes`: pending plans with exact before/after (PlanReview), passkey approval, apply, discard, recent runs;
      top bar counts plans awaiting approval.
- [x] `/dns` Records: add/edit/delete entries inline (editor from the mock), "Save · creates a plan", pending tag;
      Import: saving creates the prune plan; "Test write access" on `/dns`; optional write test in setup step 3.
- [x] E2E against the fake: write test in setup, entry created → applied and verified, delete → Pending changes →
      passkey approval → gone; NS/SOA have no edit button.
- [x] Record gate against the real zone (spec row S3, as amended): `TestRealZoneRecordPath` (`-tags 'dev realzone'`),
      2026-10-28 on `s3-dns-ui`: 58 RRSets before; `_hashi-test` A 192.0.2.10 TTL 300 saved → plan applied "1 created,
      verified", direct read shows it with the owner label; delete → high-risk plan pending (still at the provider),
      approve refused without reauth, approved after a passkey assertion → "1 deleted, verified"; 58 RRSets after,
      NS hash and SOA (without the provider's serial) unchanged.
- [x] Deploy (user OK, `/deploy 94d60ad`) and prod-proof on the deployed install: write test through setup step 3's
      approved plan (probe TXT created + deleted); NS/SOA unchanged.

Decisions made in PR B:
- RRSets Hashi created (origin user/system/managed) are left off the import table: there is nothing to import.
- The setup write test shows its plan and runs on "Approve and run" (no passkey exists in setup yet).
- Saving import decisions (setup or `/dns`) plans the removals on the server, so "Remove" chosen in setup waits on
  Pending changes (review #31). The pending-changes counter is always in the top bar, amber when something waits.
- Review #31 fixed (reproduced): a save reported another entry's waiting delete as its own; a TTL typo became "zone
  default" without a preview; setup's removals never became a plan.

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

**S2 — DNS read + import** · status: **done** 2026-10-28, deployed `04ff8c7` (#28, #29), prod-proof passed.

Scope and gate: 11-delivery.md §3 row S2 — 03-dns.md §1–4 (read side), setup step 3 (02 §1.3). Gate: the real zone is
listed; the import table shows the real RRSets; import decisions persist; sanitized fixtures committed.

External systems: Hetzner Cloud API (DNS zones and RRSets). No contract yet → captured first (read-only).

Contract
- [x] `make contract-capture PROVIDER=hetzner`: list zones (paginated), get zone, list RRSets (paginated), bad token
      (401), unknown zone (404); raw into `.contracts-raw/`, sanitized into `testdata/contracts/hetzner/` with
      `captured_at` + docs URL; denylist check. Token: V1's project token copied read-only into the keychain
      (`hashi-hetzner-token`) on the user's request. Record names, external hosts and TXT contents are sanitized too.

PR A — backend
- [x] Migration 00003: DNS provider connection (type, default TTL, token as vault secret), `dns_zones`,
      `dns_observed_rrsets`, `dns_ownership`, `dns_import_decisions`; sqlc queries.
- [x] `internal/dns`: provider interface (read part: Capabilities, ListZones, ResolveZone, ListRRSets), RRSet model and
      normalization (TXT, TTL default); `internal/dns/hetzner` read adapter from the fixtures (pagination, RateLimit
      backoff, legacy-token error); replay tests; `TestInvariant_2_*` (NS/SOA never selectable for prune) on the import side.
- [x] Zone refresh job (observed snapshot) + OpenAPI: `GET /api/dns/zones`, `POST …/refresh`, `GET …/rrsets`,
      `GET /api/dns/import/{zoneId}`, `PUT …/decisions`; setup step 3 endpoints; `make generate`.

Decisions made in PR A:
- Setup step 3 has its own endpoints (`PUT /api/setup/dns/provider`, `GET|PUT /api/setup/dns/import`, scope
  `setup.manage`), because bootstrap sessions carry no admin scopes; they share the service with `/api/dns/*`.
- Step 3 stays `built: false` until PR B adds its UI and the E2E run covers it (flipping it now would break the E2E
  setup flow, which cannot reach step 3 without a page). `make dev`/E2E then need a fixture-backed fake provider.
- The observed snapshot stores TXT values decoded (chunks joined, RFC 1035 escapes resolved); re-chunking for writes is
  S3's job, from the write capture. `ttl: null` stays "zone default".
- NS/SOA and change-protected RRSets allow only "leave" on the import table; the schema backs this with CHECKs.
- `dns.refresh` job interval = the sync interval at startup (a changed interval applies after a restart).
- No 429 response was captured; the adapter acts on the status code and the captured `RateLimit-*` headers only.
- SRV and CAA are managed types (user's request, spec 03 §3). Hetzner supports 16 types; the others are leave-only.
- Write contract captured early on the user's OK (2026-10-28): `tools/contract-capture/hetzner_write.py` creates,
  changes, labels and deletes probe RRSets named `_hashi-test` / `*._hashi-test` of every managed type, plus the error
  cases; fixtures in `testdata/contracts/hetzner/write/`, nothing left in the zone. S3 builds the write side from these.
- PR A review (#28) fixed: a token that cannot see the stored zone no longer replaces the stored token (no zone chosen
  → nothing stored); types outside the provider's managed set are leave-only on the import table.

PR B — UI + deploy
- [x] Setup step 3 (provider, token, default TTL, zone auto-pick or choice, read-only validation) and the import
      table (shared component); step 3 built; E2E covers it against the fixtures.
- [x] `/dns`: Records (observed, origin, read-only in S2) and Import tab with decisions, from the approved mock.
- [x] Contract drift job (03 §2 point 4): `.gitea/workflows/contract-drift.yml` (manual dispatch + Mondays) and
      `make contract-drift`; compares JSON paths/types only. Repo secret `HETZNER_TOKEN` set (the existing project token, user's choice).
- [x] Deploy (user OK) and prod-proof: real zone listed, import table matches the zone, decisions survive a restart.

Decisions made in PR B:
- `-tags dev` builds serve recorded fixtures in-process when `HASHI_DEV_DNS_FIXTURES` is set (`make dev`, E2E; token
  `replay-token`, zone `example.com`); release builds contain no fixture code (`devDNSProviders` stub).
- Saving the import table sends a decision for every row (new rows as `leave`), so a saved table has no "new" rows.
- The default TTL field starts at 3600 s (no spec value).
- The import table hides "Remove" on RRSets Hashi owns other than imports (the server rejects it too).
- The drift diff merges pages per endpoint and does not look inside `labels`, so a grown zone or new labels are no
  drift; a path that appears or disappears, or a value type the fixtures never had, is (review #29: labels would
  otherwise have failed the job from S3 on).
- E2E uses a root domain outside the fixture zones, so it always walks the zone choice; auto-pick stays covered by the
  Go integration tests. On a re-import the table starts with "New only" on (03 §4).

**S1 — Setup, auth, vault** · done, signed off by the user 2026-10-27, deployed `8e5c1b5`.

Scope and gate: 11-delivery.md §3 row S1. Pages built on the approved mocks (`/setup`, `/login`, `/settings`, `/activity`)
following `docs/spec/10b-ui-system.md`.

External systems: none (WebAuthn via `go-webauthn`, tested with a Playwright virtual authenticator), so no contract capture.

PR A — backend (#25, merged)
- [x] Migration 00002: setup state, audit, admin user, bootstrap credentials, passkeys (with RP ID), sessions, secrets,
      recovery key, recovery links; sqlc queries; S0 noop job removed.
- [x] Vault (service mode): master key file required (`HASHI_MASTER_KEY_FILE`, `hashi keygen`), HKDF KEKs, per-secret DEK,
      key-check canary; recovery key (grouped base32 + checksum) wraps the master key; `hashi vault recover`.
- [x] Client-IP resolver (01 §8), audit log, settings framework (General, Security, Appearance), live-log ring.
- [x] Sessions (02 §3): sha256-only storage, strict IP binding, 4 h idle / 8 h absolute, reauth on the row, CSRF, cleanup
      job; scope table generated from `x-hashi-scopes/-session/-reauth`; 02 §6 tests over real HTTP.
- [x] Bootstrap (argon2id, printed once, private networks, `hashi bootstrap reset`); passkeys (RP ID = root domain, origin
      = admin domain); `hashi admin recover` link; setup steps 1, 2, 9, 11 with invariant 14.

PR B — UI + deploy (#26)
- [x] Live AppShell (session, sign-out, built pages only), CSRF in the client, reauth-and-retry, 401 → sign-in.
- [x] `/setup`, `/login`, `/login/recover`, `/settings` (Account, General, Security, Appearance), `/activity`.
- [x] `make test-e2e` + CI job: full setup with a virtual authenticator → passkey sign-in → account → reauth → sign-out →
      sign-in again → deleting the session's passkey ends it; console recovery link.
- [x] Compose: master key as a Docker secret; deploy.sh refuses to start without it; `make dev` uses a dev build + dev key.
- [x] Master key created on the host by the user; deployed `8e5c1b5` (user OK); prod-proof below.

Decisions made in S1:
- `-tags dev` builds (never in images) allow a `localhost` passkey origin and count later-slice setup steps as done, so
  `make dev` and E2E can run the whole flow before S5 brings HTTPS.
- Setup cannot finish in production builds until S2–S6 add steps 3–8; the deployed S1 only reaches step 9's prompt (no
  HTTPS for passkeys yet). That is expected.
- Deferred: `hashi vault rotate` (02 §5.2) → S12 with the backup drill; AAGUID → authenticator name (needs a metadata
  source) → later; settings without a consumer yet (sync interval → S2, internal URL → S5, theme already applied by the UI).

Spec changes before S1 (2026-10-27, from an external review, agreed with the user): S1 tests passkeys with a virtual
authenticator only (no HTTPS before S5; first real passkey in S5, RP ID = root domain); setup steps arrive with their
slices; Hashi issues and distributes certificates itself (03 §5.3, no `httpreq`); generated DNS is IPv4-only; forward-auth
fallback guarantees stated and proven in S4; decision-order table with overlap rule (06 §3.1); firewall rollback warns
on reverted foreign rules; security telemetry bounded by volume (01 §4.3, 06 §5.3–5.4).

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
| S2 DNS read + import | **done** | 2026-10-28 · `04ff8c7` · `make deploy ENV=prod TAG=04ff8c7`: readyz ok, version `04ff8c7`, SPA/public 200. `hashi bootstrap reset` in the container, bootstrap login from the laptop (seen as `10.0.4.1`, scopes `setup.manage`); base settings with the real root domain. Step 3 with the real project token: the zone containing the root domain picked automatically (token sees 2 zones, the other ignored), provider `healthy`, 58 RRSets read; no write requests. Import table vs a direct read-only Hetzner read: 58 = 58 RRSets, 0 differences in name/type/TTL/values/protection; only NS and SOA leave-only. All 58 saved as `leave` (no takeover before the V1 cutover) → step 3 `done`; `docker restart hashi-v3` → 58 decisions still `leave`, 0 new, step 3 still done; V1 container untouched. Screenshot of the setup page shown to the user. Drift check: no drift against the live API. |
| S3 DNS write | **done** | 2026-10-28 · `94d60ad` · `make deploy ENV=prod TAG=94d60ad`: migration 3 → 4 with pre-migration backup, readyz ok, version `94d60ad`, SPA/public 200, container healthy, V1 untouched. Direct Hetzner read before: 58 RRSets, NS hash `d604b402d218`, SOA without serial `131e884f3662`, no `_hashi-test`. `hashi bootstrap reset`, setup login from the laptop (scope `setup.manage`); `POST /api/setup/dns/probe` → plan pending, high risk, 2 items (create + delete `_hashi-test` TXT), zone unchanged while pending; `POST …/probe/{id}/run` → applied in 14.6 s: "write access confirmed: _hashi-test TXT created, verified and deleted". Direct read after: 58 RRSets, NS and SOA (without serial) unchanged, no `_hashi-test`. Record path (spec row S3 as amended): `TestRealZoneRecordPath` against the real zone, same code (see PR B checklist): create applied + verified with owner label, delete pending until passkey reauth, then deleted and verified; zone unchanged. First background reconcile on the server (12:41 UTC): `dns.refresh` succeeded, "juzo.io: in sync" (no plan). Bootstrap credentials file deleted after use. |
| S4 SSH + Traefik | in progress | |
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

- S3 PR B review (#31): discarding a waiting entry-delete plan on `/changes` probably does not last: the entry is
  already gone from Hashi, so the next save or hourly sync proposes the same delete again. Not demonstrated; the spec
  does not say what Discard means there (undo the entry removal, or just "not now").

- S2 PR B review (#29): `/dns` "show generated" hides only `system` RRSets; the mock also counts `managed` as
  generated. No S2 row has either origin, so it cannot be observed yet — recheck when S3 creates records.

- S2 PR A review (#28): the zones pagination replay uses captures taken with `per_page=1` while the adapter sends
  `per_page=25` (the replay ignores `per_page`); `zone_get_by_name.json` is never replayed because the adapter does not
  call that endpoint. Test-shape gaps, no wrong behaviour observed.

- S1 review (#26): Activity → Live log keeps its last sequence number across a server restart (sequence restarts at 0),
  so an open tab could skip the startup lines after a restart. Read from code, not observed.
- S0 review: for a `v1.2.3` git tag the image tag is `1.2.3` (metadata-action `{{version}}`) but the binary reports
  `v1.2.3`, so `make deploy TAG=1.2.3` would fail its smoke version match (`deploy/deploy.sh` smoke). Not observed;
  only short-SHA deploys exist so far.

## Log

- 2026-10-28 — S4: two more temporary dev targets on host B, `hashi-edge-debian` (Debian 13) and `hashi-edge-fedora`
  (Fedora 44, template downloaded for it), same size, no autostart, sshd with the laptop key, reachable from the Hashi
  container. Fedora (dnf + systemd) added to the supported host OS list at the user's request.

- 2026-10-28 — S4 prep: the live Traefik instances on both hosts stay untouched (replaced, not adopted, 11-delivery.md §7).
  A temporary dev target exists for S4: a fresh unprivileged Alpine 3.23 LXC `hashi-edge-test` on host B (1 CPU,
  512 MB, 4 GB, no autostart, sshd with the laptop key, reachable from the Hashi container). It is not the production
  edge: base image, name, address and size of the real edge LXCs are decided with the user at the V1 cutover, then this one
  is deleted.

- 2026-10-28 — S3 done: #30 (backend) and #31 (UI) merged, deployed `94d60ad`, prod-proof passed (see the S3 row).
  README bar 5/15. Next: S4 (SSH + Traefik). Admin pages on the server need S5's HTTPS for passkeys.

- 2026-10-28 — S2 done: PR B (#29) merged, deployed `04ff8c7`, prod-proof passed (see the S2 row). README bar 4/15.
  Next: S3 (DNS write). Which existing records Hashi takes over is the user's decision at the V1 cutover.

- 2026-10-28 — S2 PR A (backend) done on `s2-dns-read`: migration 00003, `internal/dns` + Hetzner read adapter from the
  fixtures, refresh job, DNS and setup step 3 API. PR B (UI, deploy, prod-proof) next.

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
