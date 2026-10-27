# Progress

The single place that says where the project stands. Updated at the end of every working session.
Rules: docs/spec/11-delivery.md §2.

## Current slice

**S0 — Walking skeleton** · status: in progress

Gate: container runs on the Docker LXC next to V1; `/readyz` OK from LAN; CI green on Gitea runners; image published.

PR A — backend skeleton (`feat/s0-backend`)
- [ ] `go.mod` (toolchain pinned, codegen/lint tools via `tool` directives), `cmd/hashi`, `internal/app` config from env + flags
- [ ] Three listeners (admin 8080, public apps 8081, public status 8082; each configurable, public ones disableable)
- [ ] `log/slog` JSON + redacting handler, `secrets.Value` type (invariant 5 test)
- [ ] SQLite (WAL, foreign_keys, busy_timeout; one writer + read pool), goose embedded migrations, pre-migration backup
- [ ] Job runner skeleton: `jobs` / `job_runs`, no self-overlap, one `noop` kind (invariant 15 fields)
- [ ] `/healthz` (process up) and `/readyz` (DB ping + migrations at head)
- [ ] `Makefile`: `check`, `generate`, `build`, `test`

PR B — API + SPA
- [ ] `api/openapi.yaml` (3.1) with `GET /api/version`; oapi-codegen strict server; `x-hashi-scopes` declared check
- [ ] `web/` SvelteKit 2 + Svelte 5 + adapter-static + Tailwind 4 + shadcn-svelte init; shell page calls `/api/version` via generated client
- [ ] SPA embedded in the binary with SPA fallback; `make dev`; web check/lint/unit in `make check`

PR C — image, CI, deploy
- [ ] `deploy/docker/Dockerfile` (BuildKit, `$BUILDPLATFORM`, cross-compile, distroless), `deploy/compose/` + `.env.example`
- [ ] `.gitea/workflows/ci.yml` (make check) and `release.yml` (amd64+arm64 → `git.juzo.io/juzo/hashi`, `latest` + short SHA)
- [ ] CI check asserting Dockerfile uses `$BUILDPLATFORM`/`TARGETARCH` and workflow keeps both platforms
- [ ] `make deploy ENV=prod TAG=…`, `make rollback`, `make smoke`
- [ ] Deploy to CT 106 `hashi-v3` on 3340–3342 (needs user OK: writes files + pulls image on a prod host) → gate evidence

## Slices

| Slice | Status | Prod-proof (date · version · evidence) |
| --- | --- | --- |
| S0 Walking skeleton | not started | |
| S0b UI design for all features | — | |
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

## Log

- 2026-10-27 — Restart. V2 attempt removed from history (backup bundle kept off-repo). New spec in `docs/spec/`.
