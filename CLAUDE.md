# Hashi

Single-user homelab edge control plane: Hetzner DNS, Traefik, Linux firewall hosts (iptables/ipset + NetBird),
AdGuard rewrites, monitoring, public status/app pages, edge SSO, abuse engine, Pulse agent.
Go backend + embedded SvelteKit SPA + SQLite, one container. Pulse agent in `agents/pulse` (Go).

## Where things are

- Spec: `docs/spec/README.md` (index + invariants). Read only the area file you work on.
- Where we are: `docs/progress.md` — read it first in every session, update it last.
- Decisions: `docs/adr/`. Real infrastructure facts: `CLAUDE.local.md` (gitignored, not in the public repo).
- `legacy/v1/` is the old Node app still running production until cutover. Don't copy its patterns.

## Commands

- `make check` — fmt, lint, unit + contract tests, generated-code diff, web checks. Run before saying anything is done.
- `make generate` — after editing `api/openapi.yaml` or any `.sql` query.
- `make test-integration` — container-based tests (Traefik, sshd, iptables netns, AdGuard).
- `make contract-capture PROVIDER=x` — record real API responses (read-only) into `testdata/contracts/`.
- `make dev` — backend + Vite dev server with fake providers.
(The Makefile is created in slice S0; until then these don't exist.)

## How we work (the reasons are in docs/spec/11-delivery.md)

- One slice at a time. A slice is done only after it worked against the real system and the evidence is in `docs/progress.md`.
- Never write an adapter, CLI invocation or file format from memory. Capture it from the real system first, or stop and ask.
- Bug fix = reproduction first (failing test or recorded real failure). Two failed attempts on the same failure → stop,
  write hypothesis + evidence under "Blocked" in progress.md, ask.
- Reviews check the diff against the spec section's acceptance criteria, skip style, max 10 findings. Unreproduced
  findings go to "Unverified", not into code. Never start an audit series.
- PRs are coherent and reviewable: usually one or two per slice, split along natural seams, rough guide ~1,500
  hand-written lines. No stacked micro-PRs (each one costs a CI run). Docs and small fixes go straight to `main`.
- If the spec is wrong, fix the spec in its own `docs(spec):` commit with the reason. Don't silently diverge.
- Conventional commits; migrations ship with the model change; OpenAPI and generated clients ship together.

## Skills

- Project skills in `.claude/skills/`: `slice` (drives the work), `contract-capture`, `review`, `debug`, `infra-safety`, `deploy`.
- Plugins enabled for this repo: `frontend-design` (use for every UI page/component, within the tokens in
  `docs/spec/10-ui.md`) and `superpowers` (brainstorming, plans, TDD, systematic debugging, verification). Superpowers
  skills are tools inside the slice process; where they conflict, `docs/spec/11-delivery.md` wins (plans live in
  `docs/progress.md`, not separate plan files).

## Safety

- The repo is public. No tokens, real hostnames/IPs, production configs or unsanitized fixtures in commits.
- These rules are for you, the coding agent. Deployed Hashi runs its remote commands unattended by design (plan → apply → verify).
- Production hosts: read-only commands unless the user approved the exact change in this session. Never flush
  iptables/nft chains, never touch NetBird rules, never restart Proxmox, Traefik or DNS services without approval.
  Use the `infra-safety` skill before running anything on a remote host.
- Hashi's own invariants (NS/SOA untouchable, owned-objects-only, plan→apply→verify) are in `docs/spec/README.md` and have tests named `TestInvariant_*`. Never weaken those tests.

## Code

- Go: stdlib-first, `log/slog` with the redacting handler, secrets only as `secrets.Value`, errors wrapped with context,
  no global state except in `internal/app` wiring. Renderers marshal structs; never string-template YAML or shell from user input.
- Web: Svelte 5 runes, shadcn-svelte/Bits UI only, semantic tokens only (no hex in components), API types from the generated client.
