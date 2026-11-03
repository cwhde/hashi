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
| 14 | function | seen | Pulse agent command line | `hashi-pulse --version` (and `version`) does not print the version: it starts a second agent beside the running one, whose heartbeat Hashi answers with 429. Seen on an edge container 2026-11-02; the extra process was stopped by hand. | 2026-11-02 |
| 15 | visual | seen | Service page header on a phone | At about 390 px wide the service name runs under the "Turn off" button in the header card (seen in a browser-test screenshot, 2026-11-02). |
| 16 | later | seen | CI, `internal/edge` | `TestIntegrationPocketID` panicked at its setup: Pocket ID v2.19.1 answered the OIDC client creation without `createdSecret` (main run 1131 after `77bded2`). The same code passed on the PR and three times locally. Not reproduced; maybe Pocket ID not fully started. | 2026-11-02 |
| 17 | later | reported | Services → target | Giving a service a Pulse agent as its target takes a detour: first an IP set for the agent, then that set in the service. Pick the agent directly in the service instead. | 2026-11-03 |
| 18 | later | reported | Service → WAF → exclusions | Typing bare rule numbers (920440, …) is unintuitive. Show each rule as a short readable name and offer a search by that name. The names should come from the rules themselves (every CRS rule carries a `msg` in the embedded rule files), not from a list we keep by hand. | 2026-11-03 |
| 19 | function | seen | Service with a Pulse agent as target | The Traefik backend gets every internal address of the agent (LAN, an IPv6 ULA, the Docker bridge 172.17.0.1), not just the selected one, so Traefik spreads requests over three addresses and two of them fail. Firewall sets want all addresses; a service target wants the selected one. Seen 2026-11-03 on a service on the home server. | 2026-11-03 |
| 20 | function | seen | Service behind the home server | The home server isn't a host in Hashi (only the VPS is), so for a service whose target is at home Hashi can't tell which entry it sits behind and silently creates neither a public record nor an AdGuard rewrite. Either the home server becomes an `external_nat` host (05 §1.1), or the service page must say why nothing is published. | 2026-11-03 |
| 9 | later | seen | Traefik install plan | One failed download of the Traefik release (network timeout) fails the whole plan; there is no retry, so an install during an edge replacement extends the outage until someone runs it again. | 2026-11-01 |
