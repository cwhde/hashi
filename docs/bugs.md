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
| 150 | function | seen | The VPS's `/etc/hosts` | The pin line for the NetBird server's name holds a DNS error message (";; communications error to …#53: timed out") where the address should be; probably written by the old firewall script before the takeover. Seen read-only 2026-11-19; harmless (a line starting with `;` is not an address) but stale. Remove with the owner's OK. | 2026-11-19 |
| 151 | function | seen | Changes → Recent runs | While the page loads, the panel says "No plan has finished yet" for a moment before the list arrives; it should show a placeholder until loaded. Seen on prod 2026-11-19 with 0.13.5-beta.1. | 2026-11-19 |
| 152 | a11y | seen | IP set picker | Its search field has no accessible name (the app shell search has one since 13.7): Bits UI's command input points `aria-labelledby` at a hidden label taken from `Command.Root`'s `label`, which they leave empty, so screen readers (and Playwright's `getByLabel`) find a nameless field. Fix: `label` on each `Command.Root` (done for the WAF rule picker, bug 18). Found 2026-11-20. | 2026-11-20 |
| 153 | function | seen | Abuse engine → challenge to ban | An app that cannot show a CAPTCHA (the owner's photo app) is banned one second after its challenge: its parallel requests use up `challenge.max_failed` (15) at once. Seen on prod 2026-11-20 08:15:44–45 UTC (request limit still 300; the phone tripped it). The challenge-to-ban step needs time, not only a count, before it bans. | 2026-11-20 |

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
