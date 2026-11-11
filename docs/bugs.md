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
| 11 | later | seen | CI, `internal/firewall` | `TestIntegrationRescueRollback` hung in `Apply` on a `docker exec` until the 10 min test timeout (PR #55, run 1110, first attempt); the rerun passed, and so does a local run. Probably the backgrounded rescue timer keeping the exec's output open on that runner. Not reproduced. | 2026-11-02 |
| 12 | later | reported | Alerts | Group alerts: when almost all services go down at once (one host or the edge is down), send one grouped alert instead of one per service, and plan how to avoid the flood. | 2026-11-02 |
| 14 | function | seen | Pulse agent command line | `hashi-pulse --version` (and `version`) does not print the version: it starts a second agent beside the running one, whose heartbeat Hashi answers with 429. Seen on an edge container 2026-11-02; the extra process was stopped by hand. | 2026-11-02 |
| 15 | visual | seen | Service page header on a phone | At about 390 px wide the service name runs under the "Turn off" button in the header card (seen in a browser-test screenshot, 2026-11-02). |
| 16 | later | seen | CI, `internal/edge` | `TestIntegrationPocketID` panicked at its setup: Pocket ID v2.19.1 answered the OIDC client creation without `createdSecret` (main run 1131 after `77bded2`). The same code passed on the PR and three times locally. Not reproduced; maybe Pocket ID not fully started. | 2026-11-02 |
| 17 | later | reported | Services → target | Giving a service a Pulse agent as its target takes a detour: first an IP set for the agent, then that set in the service. Pick the agent directly in the service instead. | 2026-11-03 |
| 18 | later | reported | Service → WAF → exclusions | Typing bare rule numbers (920440, …) is unintuitive. Show each rule as a short readable name and offer a search by that name. The names should come from the rules themselves (every CRS rule carries a `msg` in the embedded rule files), not from a list we keep by hand. | 2026-11-03 |
| 20 | function | seen | Service behind the home server | The home server wasn't a host in Hashi (only the VPS is), so for a service whose target is at home Hashi can't tell which entry it sits behind and silently creates neither a public record nor an AdGuard rewrite. Explains why so few home services have rewrites. Add host now offers "behind a router" with its Traefik and entry host; left: the service page must say why nothing is published when no host matches. | 2026-11-03 |
| 21 | later | reported | Pulse agent on OpenWrt | Pulse should run on OpenWrt routers (and that kind of system in general: BusyBox, musl, procd instead of systemd/OpenRC), on arm64 and x86-64. The static binaries probably start already; install, service start (procd init script), self-update and address detection are untested there. Before 1.0. | 2026-11-03 |
| 22 | later | reported | Pulse agent page | Show the addresses an agent detected (per interface) and let the admin pick its main address, which services and DNS then use. Today the agent picks it and the page doesn't show the choice. | 2026-11-03 |
| 23 | function | reported | Firewall host behind a router | Its public address can only be typed in. The spec (05 §1.1) says it comes from the host's egress or from a Pulse agent, or is set by hand; the agent option was never built. A home address that changes then leaves the host records stale. Pick a Pulse agent as the source of the public address (and of the host's networks). | 2026-11-03 |
| 24 | function | reported | Connections page vs. the rest | We built a Connections page on purpose for every kind of connection, yet Traefik hosts, firewall hosts and Pulse agents are each added and configured on their own pages with their own forms. Redesign how things are set up and linked: add and configure all connections (reverse proxies, firewall hosts, Pulse agents, …) from the Connections page, and let services, hosts and sets reference them directly (see also 17, 22, 23). | 2026-11-03 |
| 25 | function | in PR (branch `claude/user-middlewares-84c424`, not merged, not deployed) | Services → traffic options | User middlewares are in the spec (04 §1, §3: `30-user-middlewares.yml` with an editor, extra middlewares per service and route) and were cut in S4 for "before the V1 cutover", then never built. Build them, and expose more traffic options per service in the normal screens: the Host header sent upstream (keep, or set e.g. the target's own name, for apps that check it), forwarded client-IP headers (`X-Forwarded-For`, `X-Real-IP`), extra request/response headers, timeouts. Seen 2026-11-03: an app behind Hashi answered 403 to the public host name and there was no way to change what it receives. | 2026-11-03 |
| 26 | later | reported | Abuse engine → thresholds | Measure the thresholds (L7 rate, L4 meters, WAF count) against realistic clients, e.g. simulated Windows devices and browsers that open many connections and requests in the background, and tune the defaults so normal use never trips a challenge or a ban. | 2026-11-11 |
| 27 | later | reported | Code, everywhere | Check for implicit state machines (states spread over several booleans or strings) and, where it helps, turn them into one explicit enum. | 2026-11-11 |
| 28 | visual | reported | UI, all pages | Eyebrow optimisation: review the small labels above headings and cards; keep the ones that carry information, drop or rework the rest. | 2026-11-11 |
| 30 | function | reported | Public pages → assets | The asset system of 07 §9 (default icons, logos and backgrounds downloaded once to /data/assets, replaceable in Settings → Appearance) was never built in S7. Until it exists the Appearance settings for logo, icon and public page assets have nothing to change. | 2026-11-11 |
| 31 | function | reported | Overview page | The overview page from the design mock (widgets: pending changes, needs attention, service health, …) is not built, so the Appearance setting for widget order and visibility has nothing to change. | 2026-11-11 |
| 9 | later | seen | Traefik install plan | One failed download of the Traefik release (network timeout) fails the whole plan; there is no retry, so an install during an edge replacement extends the outage until someone runs it again. | 2026-11-01 |
