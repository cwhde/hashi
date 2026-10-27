---
name: contract-capture
description: Record and sanitize real responses from an external API, CLI or file format (Hetzner Cloud DNS, AdGuard Home, Traefik API/CLI, iptables-save/ipset/nft output, NetBird status, Cap, Telegram, Discord, OIDC, MaxMind) before writing or changing an adapter or parser. Use whenever code talks to something outside Hashi.
---

# Capture a contract from the real system

Hashi V2 failed because adapters were written from memory (shut-down Hetzner API, a nonexistent `traefik check`,
NAT chains in the wrong table). Every external interface is captured first.

1. Identify the exact calls/commands the feature needs, and the official docs URL for each.
2. Use **read-only** calls only (GET, `list`, `status`, `-save`, `--version`). For a write contract, propose a disposable
   probe (e.g. `_hashi-test` TXT record, a temp file under `/var/lib/hashi/`) and get the user's explicit OK first.
3. Credentials come from the local environment or the user — never paste them into files in the repo.
4. Capture with `make contract-capture PROVIDER=<name>` (or by hand if the target isn't automated yet), raw output into
   `.contracts-raw/` (gitignored).
5. Sanitize into `testdata/contracts/<provider>/<call>.json|txt`: replace tokens, account IDs, real domains (→ `example.com`),
   IPs (→ `192.0.2.x`, `198.51.100.x`, `2001:db8::`). Keep structure, field names, types, pagination and error shapes exactly.
   Add a header: `captured_at`, `docs_url`, version of the remote software.
6. Capture at least one error response (bad auth, not found) and one paginated response where applicable.
7. Write replay tests that feed the fixtures through the adapter/parser. Record the capture in `docs/spec/environment.md`
   ("Contract captured" column) and any surprise under "Constraints discovered".

If the real system behaves differently from the spec, update the spec (separate `docs(spec):` commit) — reality wins.
