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
