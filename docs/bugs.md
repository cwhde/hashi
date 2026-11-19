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
| 12 | function | reported | Alerts | Group alerts: when almost all services go down at once (one host or the edge is down), send one grouped alert instead of one per service, and plan how to avoid the flood. Proposal: when 3 or more checks behind the same Traefik or firewall host go down within 2 minutes, send one alert naming the host and the count, hold the single alerts for those checks, and send one recovery alert. A host check that is down itself is the cause line. | 2026-11-02 |
| 18 | later | reported | Service → WAF → exclusions | Typing bare rule numbers (920440, …) is unintuitive. Show each rule as a short readable name and offer a search by that name. The names should come from the rules themselves (every CRS rule carries a `msg` in the embedded rule files), not from a list we keep by hand. | 2026-11-03 |
| 21 | later | reported | Pulse agent on OpenWrt | Pulse should run on OpenWrt routers (and that kind of system in general: BusyBox, musl, procd instead of systemd/OpenRC), on arm64 and x86-64. The static binaries probably start already; install, service start (procd init script), self-update and address detection are untested there. Before 1.0; proven on the owner's own OpenWrt router (owner, 2026-11-16). | 2026-11-03 |
| 26 | later | reported | Abuse engine → thresholds | Measure the thresholds (L7 rate, L4 meters, WAF count) against realistic clients, e.g. simulated Windows devices and browsers that open many connections and requests in the background, and tune the defaults so normal use never trips a challenge or a ban. Method: record 10 minutes of real traffic through the edge from a browser on the heaviest services (photo gallery, home automation) and from Windows and macOS background traffic; replay it against the dev target; defaults must leave at least 3× headroom over the busiest normal client. | 2026-11-11 |
| 30 | function | reported | Public pages → assets | The asset system of 07 §9 (default icons, logos and backgrounds downloaded once to /data/assets, replaceable in Settings → Appearance) was never built in S7. Until it exists the Appearance settings for logo, icon and public page assets have nothing to change. | 2026-11-11 |
| 34 | function | seen | Firewall plan on the VPS | A feed added 129.73.0.0/16 to `block4`; the high-risk plan "block4 newly blocks 129.73.0.0/16" was approved during the S12 restore drill (07:28 UTC 2026-11-13), applied, and the check afterwards said the host can no longer reach NetBird's management server, so the rescue timer put the old table back at 07:31:20 (worked as designed). Why blocking that range breaks the NetBird check is not known yet; the same plan waits unapproved on prod. Reproduce on the dev target before touching it. | 2026-11-13 |
| 35 | function | reproduced | Changes → approve | The owner didn't know what the high-risk firewall plan would do and approved it; the approval dialog doesn't say plainly what is at stake (which addresses get blocked, what could stop working, that the rescue timer will undo a failed check). | 2026-11-13 |
| 59 | function | reproduced | Status → a check → Pause / Resume | Saves the whole form, including edits not yet saved (`save({ ...s, paused })` with the draft `s`). Codex changed Group without saving, pressed Pause, reloaded: the new group was stored. Pause should only send paused. (Codex F-020) | 2026-11-14 |
| 76 | function | reported | Service save message | "1 plan(s) need your approval" after every service save, while the only waiting plan is the unrelated firewall plan from before. The save handler adds every plan the reconcile returns, including ones that were already waiting (`services_handlers.go`, `sync/engine.go` returns the existing plan when nothing changed). Count only plans this save created or changed. Also "plan(s)". (Codex R-003) | 2026-11-15 |
| 77 | function | reported | TCP/UDP service saved while the firewall plan waits | List and detail say "Published" / "Published on every Traefik host" although both hosts' plans still wait and nothing reached the hosts. Published is derived from enabled + valid only. (Codex R-007) | 2026-11-15 |
| 79 | function | reproduced | Public app dashboard header | "6 / 7 hosts online" counts app cards, not hosts: showing one more service makes it "7 / 8" (`monitor/views.go` adds one per card, and counts Degraded as online). (Codex R-005, checked in code) | 2026-11-15 |
| 80 | function | reproduced | Public status page with nothing public | Says "All services are working." above "Nothing is listed here yet." (zero down = all working, `PublicStatus.svelte`); "Updated" also comes from all checks, not only public ones. (Codex R-006) | 2026-11-15 |
| 81 | function | reported | Security → Counters, invalid search | After the error the old 213 rows, the selected row and "Reset counters of 1" stay active, so a reset can act on rows from an earlier search (`CountersView.svelte` returns before clearing). (Codex R-008) | 2026-11-15 |
| 82 | function | reported | Status → check detail, two tabs | Saving in a stale tab silently overwrites what another tab saved; the whole form is sent with no version check (`status/[id]/+page.svelte`, `monitor/views.go`). Codex lost a saved group this way, also via App display. (Codex R-012) | 2026-11-15 |
| 85 | function | reported | Scripts → schedule preview | "Next" is shown in the browser's time zone without saying so, while the form says host time and the server computes UTC: 03:00 shows as 04:00. (Codex R-014) | 2026-11-15 |
| 86 | visual | reproduced | Status list and check detail | AdGuard and Pulse checks are grouped under "Other", have no source label, and the detail starts with ": its checks follow what it watches." `sourceText` in `lib/monitor.ts` lacks `adguard` and `pulse`. (Codex R-011, checked in code) | 2026-11-15 |
| 87 | visual | reported | Status → history strip | Always says "60 checks: 0 down, 0 degraded", also before the first check; the strip pads to 60 with unknown and counts those (`lib/monitor.ts`, `HistoryStrip.svelte`). (Codex R-009) | 2026-11-15 |
| 88 | visual | reported | Status → check detail → charts | 24h, 7d and 30d keep saying "Charts appear after the first full minute of checks" while 1h already has points, and 24h is the default, so a new check looks empty. (Codex R-010) | 2026-11-15 |
| 89 | later | reported | IP set → Resolved, large feeds | "Showing the first 200 of 1621 ranges." with no way to see more, although the API has a cursor and Items has "Show more". (Codex R-013) | 2026-11-15 |
| 107 | visual | reported | Status list | Once stayed on placeholder cards for more than 4 s although its calls answered within 0.6 s; loaded on the next try. Not measured further. (13.4 walk) | 2026-11-19 |
| 121 | function | reported | IP sets | Sorting by kind is done (13.4 PR C). Left: delete the sets no longer needed now that services and rules can name a Pulse agent directly; each deletion needs the owner's OK per set. (owner) | 2026-11-19 |
| 136 | later | reported | Sign-in page | Clean up the login page: a better design, and remove unnecessary text again. (owner) | 2026-11-19 |

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
