# Progress

The single place that says where the project stands. Updated at the end of every working session.
Rules: docs/spec/11-delivery.md §2.

## Current slice

**S1 — Setup, auth, vault** · status: deployed `8e5c1b5`, prod-proof passed, waiting for the user's sign-off

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
| S1 Setup, auth, vault | prod-proof passed, sign-off pending | 2026-10-27 · `8e5c1b5` · `make deploy ENV=prod TAG=8e5c1b5`: readyz ok, version `8e5c1b5`, SPA/public 200. Bootstrap login from the PVE host (LAN) → 200, `hashi.bootstrap` cookie (HttpOnly, SameSite=Strict, no Secure over HTTP), session bound to `10.0.4.1`, scopes `setup.manage` only. `docker restart hashi-v3` → same cookie still authenticated, bootstrap block not printed again. Cookie replayed from inside the Docker LXC (`10.0.4.106`) → `authenticated:false` + cookie cleared; afterwards rejected from `10.0.4.1` too; audit `session.revoked` `{bound_ip:10.0.4.1, reason:ip_mismatch}` observed ip `10.0.4.106`. DB + WAL copy: bootstrap password, master key (base64 and raw) absent; `secret_records` holds only AES-GCM ciphertext; session ids stored as 32-byte sha256; bootstrap hash argon2id. Passkeys: CI E2E with virtual authenticator green on `8e5c1b5` (no HTTPS before S5). Note: from the laptop, 10.0.4.0/24 is reached through kanae (NetBird route, SNAT), so the laptop appears as `10.0.4.1`. |
| S2 DNS read + import | — | |
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
