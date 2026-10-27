# Progress

The single place that says where the project stands. Updated at the end of every working session.
Rules: docs/spec/11-delivery.md §2.

## Current slice

**S0b — UI design for all features** · status: **waiting for the user's approval**

Gate: page inventory, navigation grouping, layout and component rules, tokens for the four themes, every page as a
clickable mock under `/design` with mock data in all four themes; approved by the user.

- [x] Rules written: `docs/spec/10b-ui-system.md` (tokens, shell, page inventory, component and writing rules)
- [x] Tokens for Graphite, Warm Linen, Verdant, Violet Night; no-flash theme loading; contrast tests for every theme
- [x] shadcn-svelte (nova) on Bits UI + Hashi components (Panel, HistoryStrip, PlanReview, AppShell, …)
- [x] Every page as a clickable mock: 24 pages incl. setup, login, public pages and the visitor challenge (#24)
- [x] Checked in the browser: all four themes at 1440 px, phone width (375 px), mobile navigation drawer
- [ ] Deploy to the Docker LXC so the user can click through (needs the user's OK)
- [ ] User approval → S1 starts

S0 — Walking skeleton · **done** (gate passed 2026-10-27, see Slices)

Gate: container runs on the Docker LXC next to V1; `/readyz` OK from LAN; CI green on Gitea runners; image published.

Code is merged (stacked PRs #17–#23, each ≤ ~500 hand-written lines). CI on the Gitea runners runs `make check` on every PR and on main.

- [x] #17 tooling: `go.mod` (toolchain pin), tools in `tools/go.mod`, `make check`, golangci-lint, `ci.yml`, `secrets.Value` + allow-list redacting slog handler (invariant 5)
- [x] #18 store: modernc SQLite (WAL, FKs, busy_timeout, one writer + read-only pool), embedded goose migrations, pre-migration `VACUUM INTO` backup restored on failure
- [x] #19 job runner: `jobs`/`job_runs`, no self-overlap, panic → failed run, orphans → interrupted, `noop` job (invariant 15)
- [x] #20 app: config (flags > env > defaults), admin/public-apps/public-status listeners, `/healthz`, `/readyz` (drains on shutdown), UTC JSON logs
- [x] #21 OpenAPI 3.1 contract, oapi-codegen strict server, test that every operation declares `x-hashi-scopes`
- [x] #22 SPA shell: Svelte 5 + SvelteKit 2 + Tailwind 4, generated TS client, embedded with index.html fallback; `make dev`; web checks in `make check`
- [x] #23 image: multi-arch Dockerfile (no emulation, distroless nonroot, ~6 MB), `release.yml` (refuses to push V1's `:old` tag), compose, `make deploy/rollback/smoke`, build-config check
- [x] Merge stack → release workflow publishes `latest` + short SHA (`e7c0a30`, amd64 + arm64, `:old` untouched)
- [x] Deploy to the Docker LXC (`hashi-v3`, ports 3340–3342), approved by the user
- [x] Gate evidence recorded in the Slices table
- [x] Spec review of the whole S0 diff: rollback lost its target on redeploy (reproduced → fixed with a stubbed-ssh test in `make check`); missing `TestInvariant_6_*` and `/readyz` 503 tests added; one unverified finding parked below

Decisions made in S0:
- SvelteKit **2** kept as specified; Kit 3.0 (2026-10-01) skipped for now — the component ecosystem still targets Kit 2.
- shadcn-svelte init moved to S0b, where the design tokens are defined.
- Deploy targets live in gitignored `deploy/targets/<env>.env` (example in `deploy/targets.example/`).
- `make dev`, `make image`, `make deploy` exist now; `make test-integration`, `make test-e2e`, `make contract-capture` arrive with the slices that need them.

## Slices

| Slice | Status | Prod-proof (date · version · evidence) |
| --- | --- | --- |
| S0 Walking skeleton | **done** | 2026-10-27 · `e7c0a30` · `make deploy ENV=prod TAG=e7c0a30` on the Docker LXC next to V1: smoke from the LAN → `/readyz` `{"checks":{"database":"ok"},"status":"ok"}`, `/api/version` `{"commit":"e7c0a30","version":"e7c0a30"}`, SPA 200, public apps 200, public status 200; container `healthy`, uid 65532, read-only rootfs; restart keeps the DB (no re-migration); V1 and Gatus still up. CI green on the Gitea runners (#17–#23 and main); release published `e7c0a30` = `latest`, amd64 + arm64. |
| S0b UI design for all features | mocks done, awaiting approval | |
| S1 Setup, auth, vault | — | |
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

- S0 review: for a `v1.2.3` git tag the image tag is `1.2.3` (metadata-action `{{version}}`) but the binary reports
  `v1.2.3`, so `make deploy TAG=1.2.3` would fail its smoke version match (`deploy/deploy.sh` smoke). Not observed;
  only short-SHA deploys exist so far.

## Log

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
