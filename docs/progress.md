# Progress

The single place that says where the project stands. Updated at the end of every working session.
Rules: docs/spec/11-delivery.md §2.

## Current slice

**S1 — Setup, auth, vault** · status: not started

Scope and gate: 11-delivery.md §3 row S1. Build pages on the approved mocks (`/design/setup`, `/design/login`,
`/design/settings` → Account, `/design/activity`) following `docs/spec/10b-ui-system.md`. First step: plan the slice as a
checklist here (`slice` skill).

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
| S1 Setup, auth, vault | next | |
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
