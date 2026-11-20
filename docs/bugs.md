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
| 4 | function | reproduced | Top bar search | Search only finds page names. The owner wants one global search for everything, since the box is on every page: every object (services, DNS records, AdGuard rewrites, IP sets and their items, addresses and CIDRs, rule sets, firewall and Traefik hosts, agents, connections, scripts, checks), every setting (by its label, opening Settings at that field), every tab of every page, and plans and audit entries. Results grouped by kind, keyboard first, each opening the exact place. Needs a search endpoint; today the address and service results exist only in the design mock (see Details). | 2026-10-31 |
| 18 | later | reported | Service → WAF → exclusions | Typing bare rule numbers (920440, …) is unintuitive. Show each rule as a short readable name and offer a search by that name. The names should come from the rules themselves (every CRS rule carries a `msg` in the embedded rule files), not from a list we keep by hand. | 2026-11-03 |
| 21 | later | reported | Pulse agent on OpenWrt | Pulse should run on OpenWrt routers (and that kind of system in general: BusyBox, musl, procd instead of systemd/OpenRC), on arm64 and x86-64. The static binaries probably start already; install, service start (procd init script), self-update and address detection are untested there. Before 1.0; proven on the owner's own OpenWrt router (owner, 2026-11-16). | 2026-11-03 |
| 26 | later | reported | Abuse engine → thresholds | Measure the thresholds (L7 rate, L4 meters, WAF count) against realistic clients, e.g. simulated Windows devices and browsers that open many connections and requests in the background, and tune the defaults so normal use never trips a challenge or a ban. Method: record 10 minutes of real traffic through the edge from a browser on the heaviest services (photo gallery, home automation) and from Windows and macOS background traffic; replay it against the dev target; defaults must leave at least 3× headroom over the busiest normal client. | 2026-11-11 |
| 30 | function | reported | Public pages → assets | The asset system of 07 §9 (default icons, logos and backgrounds downloaded once to /data/assets, replaceable in Settings → Appearance) was never built in S7. Until it exists the Appearance settings for logo, icon and public page assets have nothing to change. | 2026-11-11 |
| 81 | function | reported | Security → Counters, invalid search | After the error the old 213 rows, the selected row and "Reset counters of 1" stay active, so a reset can act on rows from an earlier search (`CountersView.svelte` returns before clearing). (Codex R-008) | 2026-11-15 |
| 89 | later | reported | IP set → Resolved, large feeds | "Showing the first 200 of 1621 ranges." with no way to see more, although the API has a cursor and Items has "Show more". (Codex R-013) | 2026-11-15 |
| 149 | function | reported | Public app dashboard → app icons | Each app card gets a real icon. Order: (1) a custom icon set on the app, either uploaded or picked from the selfh.st icon library (https://selfh.st/icons/) with a search box in the picker, like Pocket ID does; (2) otherwise fetched automatically from the app itself, the best-quality icon that fits the card (manifest icons, apple-touch-icon, favicon); if that gives none or a generic one, try again against the upstream address with the app's Host header; (3) otherwise the current letter placeholder. selfh.st's index and the fetch paths are captured from the real systems before any code. (owner) | 2026-11-19 |
| 150 | function | seen | The VPS's `/etc/hosts` | The pin line for the NetBird server's name holds a DNS error message (";; communications error to …#53: timed out") where the address should be; probably written by the old firewall script before the takeover. Seen read-only 2026-11-19; harmless (a line starting with `;` is not an address) but stale. Remove with the owner's OK. | 2026-11-19 |
| 151 | function | seen | Changes → Recent runs | While the page loads, the panel says "No plan has finished yet" for a moment before the list arrives; it should show a placeholder until loaded. Seen on prod 2026-11-19 with 0.13.5-beta.1. | 2026-11-19 |
| 152 | a11y | seen | IP set picker, app shell search | Their search fields have no accessible name: Bits UI's command input points `aria-labelledby` at a hidden label taken from `Command.Root`'s `label`, which they leave empty, so screen readers (and Playwright's `getByLabel`) find a nameless field. Fix: `label` on each `Command.Root` (done for the WAF rule picker, bug 18). Found 2026-11-20. | 2026-11-20 |

## Details

Checked 2026-11-14 on the production app (read-only: measured in the browser, nothing saved) and in the code. Pixel values
are at the app's 14 px base font.

### 4 · Search only finds pages

- **Spec:** 10b §2 promises addresses (to the address page), CIDRs, IP sets, rule sets, services, domains and pages.

- **Reproduce:** press ⌘K, type an IP address, a domain or a service name. Nothing is found.
- **What happens:** the box says "IP, CIDR, ASN, domain or page…", but in the live app it only lists the menu's page names
  (`AppShell.svelte`, `Command.Dialog`). The address and service results exist only in the design mock (`{#if !live}`),
  and there is no search endpoint in the API. So this is a missing feature presented as working, not weak results.

### 34 · Firewall plan blocking a /16 failed the NetBird check

What is known now:

- The apply that failed (07:28 UTC 2026-11-13) came from the **restored copy** of Hashi during the restore drill, not the
  production instance. Its check (`checkManagement` in `internal/firewall/manager.go`) opens a TCP connection from the
  host to the management server's name and port from `netbird status -d`; that failed within 5 s, so the rescue timer
  restored the old table.
- The management server's name resolves on the host to the host's own public address; the connection goes through
  `nat_out` (DNAT to the Traefik container) and never through the WAN interface, so a source-address block in `pre_guard`
  shouldn't touch it. The blocked range belongs to a large company unrelated to NetBird or the VPS. So the range itself
  explains nothing yet.
- `pre_guard` drops `block4` sources on the WAN before any "established" accept, so it also drops replies to connections
  the host opened itself. Not the cause here, but worth knowing.
- The "high risk" label comes from `riskWhy` in `internal/firewall/apply.go`: any new block of a /16 or wider is high risk.
  The plan's two items are whole-file and whole-table hashes; the range is only the note.
- Hypotheses to test on the dev target: (a) the restored copy rendered a different table than production would (other
  data or settings), (b) the apply briefly disturbed the DNAT path or Traefik, (c) the check ran before the new table had
  settled. Reproduce by applying the same feed change on the dev target with the check's exact command.

### 35 · The approval screen doesn't say what is at stake

What the waiting firewall plan shows today:

- The page heading says "Every change to DNS is planned first" on a firewall plan.
- "Before" and "after" are `sha256 75a2…` and `sha256 cf28…`: true, and meaningless to a person.
- The only readable line, "block4 newly blocks 129.73.0.0/16", is repeated on both items, without saying which feed added
  it, how many addresses that is (65,536), what could stop working, or that a failed check is rolled back automatically.
- The same plan is re-created every 6 hours ("superseded" five times in a row in Recent runs), so the list fills with
  noise while it waits.
- Wanted: a summary of what changes in words (sets and counts added/removed, by feed), the risk reason with its
  consequence, the rollback promise, and the hashes folded away.
