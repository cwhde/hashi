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
| 162 | function | reproduced | Public pages editor | Moving an apps-only entry (a DNS link) can take a monitored service's address: after moving the Jellyfin link, `/s/jellyfin` answers 404 and the check moves to `/s/jellyfin-2`, breaking shared links and badges. Slugs are handed out over all rows in layout order (`internal/monitor/layout.go`). | Codex round 3, 2026-11-22 |
| 165 | function | reproduced | Public status | The Last hour view shows the 24-hour uptime beside the hour's bars (Lamp: every minute down, "71.73%"). The spec says uptime for the bar's span. | Codex round 3, 2026-11-22 |
| 166 | function | reproduced | Service page | A slower answer for an earlier range replaces the chart of the range now chosen (7d then 30d: 30d selected, 7d's data shown). No check that an answer is still current. | Codex round 3, 2026-11-22 |
| 167 | function | reproduced | Public status | On a phone, tapping a history bar opens the service page instead of showing that bar's details: the strip sits inside the row's link. | Codex round 3, 2026-11-22 |
| 168 | function | reproduced | Public status | The banner says "All services are working." while a service is paused or has no data yet ("1 of 2 up" below it). | Codex round 3, 2026-11-22 |
| 169 | function | reproduced | Service page | Right after pausing, the service page says "Paused for 9 h 50 min": pausing keeps the time the last state began. | Codex round 3, 2026-11-22 |
| 170 | visual | reproduced | Public status | A group name without spaces (60 × W) pushes the group's count and dot off screen and makes the page scroll sideways (1000 px at 390, 1120 px at 1024). | Codex round 3, 2026-11-22 |
| 171 | function | reproduced | Settings → Public pages | A save with a rejected upload says only the upload's error, but the page switches and the apps page's sort were already saved. | Codex round 3, 2026-11-22 |
| 172 | function | reproduced | Public pages | A service page that is switched off or unknown answers 200 (the page shell); only its data request answers 404. The spec says 404. | Codex round 3, 2026-11-22 |
| 173 | function | reproduced | Settings → Public pages | No way to set the page titles, which the spec lists among the page-wide options (07 §8.1). | Codex round 3, 2026-11-22 |
| 175 | visual | reproduced | Service page | The charts' moment-by-moment values can only be read with a pointer; keyboard users get only the latest value (same on the admin monitor page). | Codex round 3, 2026-11-22 |
| 176 | visual | reproduced | Public pages | Screen readers hear the state twice in two words: "Slow Degraded", "No data Unknown", "Down Down": the dot keeps its admin name next to the visitor word. | Codex round 3, 2026-11-22 |
| 178 | visual | reproduced | Public pages | A first-time visitor sees Graphite until the page's data arrives, then the configured theme (Warm Linen): a dark flash before the light page. | Codex round 3, 2026-11-22 |
| 179 | visual | reproduced | Badges | Names in Japanese or other wide scripts overflow the badge: the label runs into the state value, and a long name past the badge's edge. Width is estimated per letter for Latin text only. | Codex round 3, 2026-11-22 |

## Details

Checked 2026-11-14 on the production app (read-only: measured in the browser, nothing saved) and in the code. Pixel values
are at the app's 14 px base font.

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
