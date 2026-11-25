# Bugs

Known bugs and rough edges we are not fixing right now. Bugs that block the current slice's gate don't wait here: they
are fixed in the slice (`.claude/skills/debug`). No real names, addresses or tokens: the repo is public.

How an entry works:

- Write it down rough straight away, in the reporter's words. A rough note is enough; nobody has to reproduce it first.
- Fill in the steps to reproduce when we pick it up. A fix still starts with a reproduction (CLAUDE.md).
- `Kind`: **function** (something does the wrong thing), **visual** (looks or feels wrong, works), **later** (missing
  polish, not wrong).
- `Status`: **reported** → **reproduced** → remove the entry in the commit that fixes it (git keeps the history).

Entries with more to say have a section under **Details** below the table, by number.

All open entries are planned into the 0.13 bug round (slices 13.1 onwards in `docs/progress.md`). UI fixes follow `docs/spec/10c-ui-rules.md`.

| # | Kind | Status | Where | What | Reported |
| --- | --- | --- | --- | --- | --- |
| 190 | function | mitigated | CI | `internal/app` took 420–600 s in CI (39 tests one after another, 121 s alone under `-race`); `make test` and CI allow 20 minutes. Tried 2026-11-25: running them four at a time made it 63 s locally and passed `make check`, but on CI (run 1353, the go, e2e and integration jobs share one host) instances missed the 30 s start limit, so it went back to one at a time. A fix needs instances that start fast under load, or the CI jobs not sharing one host. | 2026-11-23 |
| 199 | function | open | Public pages | Hashi itself is missing from the apps page (and its layout list), so there is no way to add it. Cause: Hashi's own admin address is a system service, and the public layout skips every system service (`internal/monitor/layout.go`, `if v.System() { continue }`), which is meant for the CAPTCHA page. Wanted: Hashi as an app entry like any other, which the owner can show or hide. | 2026-11-25 |
