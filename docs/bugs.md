# Bugs

Known bugs and rough edges we are not fixing right now. Bugs that block the current slice's gate don't wait here: they
are fixed in the slice (`.claude/skills/debug`). No real names, addresses or tokens: the repo is public.

How an entry works:

- Write it down rough straight away, in the reporter's words. A rough note is enough; nobody has to reproduce it first.
- Fill in the steps to reproduce when we pick it up. A fix still starts with a reproduction (CLAUDE.md).
- `Kind`: **function** (something does the wrong thing), **visual** (looks or feels wrong, works), **later** (missing
  polish, not wrong).
- `Status`: **reported** → **reproduced** → remove the entry in the commit that fixes it (git keeps the history).

Review them together before the V1 cutover; visual and later entries wait until the app works.

| # | Kind | Status | Where | What | Reported |
| --- | --- | --- | --- | --- | --- |
| 1 | visual | reported | Sidebar | The items move when the bar expands on hover: aiming at DNS, the cursor ends up on Services once the bar opens. Items should keep their height. | 2026-10-31 |
| 3 | visual | reported | Several pages, e.g. a "Choose a set" selector | Items in a row aren't level: the selector sits at a different height from its neighbour. Seen in several sections; find them all when fixing. | 2026-10-31 |
| 4 | later | reported | Top bar search | Search results are generally not that good. | 2026-10-31 |
| 5 | later | reported | Browser tab, logo | No favicon yet; the current icon is a good placeholder but should be replaced. | 2026-10-31 |
| 7 | visual | reported | Delete confirmation dialog | The dialog has no border, unlike other UI items in every theme. | 2026-10-31 |
| 6 | visual | reported | IP sets → add an item | The options of the first field (type) wrap onto two rows, which breaks the layout. | 2026-10-31 |
| 8 | visual | reported | All on/off switches | In the on position the switch doesn't fully slide to the right and looks broken. | 2026-10-31 |
| 10 | later | seen | CI, `internal/app` | `TestRunServesHealthAndShutsDownCleanly` waits 5 s for readiness; on a busy runner (main run of `472617a`) it timed out, while the same code passed on the PR and locally. Flaky, not reproduced. Seen again on PR #56 (run 1116, check job); the rerun was started. Third time on main after `d4d95ad` (run 1122), fourth on PR #58 (run 1128): now recurring, look at it before S10 ends. | 2026-11-02 |
| 11 | later | seen | CI, `internal/firewall` | `TestIntegrationRescueRollback` hung in `Apply` on a `docker exec` until the 10 min test timeout (PR #55, run 1110, first attempt); the rerun passed, and so does a local run. Probably the backgrounded rescue timer keeping the exec's output open on that runner. Not reproduced. | 2026-11-02 |
| 12 | later | reported | Alerts | Group alerts: when almost all services go down at once (one host or the edge is down), send one grouped alert instead of one per service, and plan how to avoid the flood. | 2026-11-02 |
| 13 | later | waiting upstream | WAF, Coraza plugin | Body checking is off (decision 2026-11-02): with body access on, the plugin loses or breaks every request body above about 160 KB (its repository's issues 8 and 15). Watch for a fixed plugin release, then reconsider offering the `enabled` mode (06 §9). | 2026-11-02 |
| 14 | function | seen | Pulse agent command line | `hashi-pulse --version` (and `version`) does not print the version: it starts a second agent beside the running one, whose heartbeat Hashi answers with 429. Seen on an edge container 2026-11-02; the extra process was stopped by hand. | 2026-11-02 |
| 15 | visual | seen | Service page header on a phone | At about 390 px wide the service name runs under the "Turn off" button in the header card (seen in a browser-test screenshot, 2026-11-02). |
| 16 | later | seen | CI, `internal/edge` | `TestIntegrationPocketID` panicked at its setup: Pocket ID v2.19.1 answered the OIDC client creation without `createdSecret` (main run 1131 after `77bded2`). The same code passed on the PR and three times locally. Not reproduced; maybe Pocket ID not fully started. | 2026-11-02 |
| 17 | function | seen | WAF on a real edge | With the WAF on every route (58 on the Frankfurt edge), Traefik did not answer ping within 30 s after the restart: the plugin builds the rulebook again for each route on a 2-core container. WAF is off by default since `v0.10.0-beta.3`; measure start time and memory per route on a dev target before switching it on again (one WAF on fewer routes, a longer start wait, or a different WAF). | 2026-11-03 |
| 9 | later | seen | Traefik install plan | One failed download of the Traefik release (network timeout) fails the whole plan; there is no retry, so an install during an edge replacement extends the outage until someone runs it again. | 2026-11-01 |
