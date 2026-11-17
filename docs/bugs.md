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
| 31 | function | reported | Overview page | The overview page from the design mock (widgets: pending changes, needs attention, service health, …) is not built, so the Appearance setting for widget order and visibility has nothing to change. | 2026-11-11 |
| 34 | function | seen | Firewall plan on the VPS | A feed added 129.73.0.0/16 to `block4`; the high-risk plan "block4 newly blocks 129.73.0.0/16" was approved during the S12 restore drill (07:28 UTC 2026-11-13), applied, and the check afterwards said the host can no longer reach NetBird's management server, so the rescue timer put the old table back at 07:31:20 (worked as designed). Why blocking that range breaks the NetBird check is not known yet; the same plan waits unapproved on prod. Reproduce on the dev target before touching it. | 2026-11-13 |
| 35 | function | reproduced | Changes → approve | The owner didn't know what the high-risk firewall plan would do and approved it; the approval dialog doesn't say plainly what is at stake (which addresses get blocked, what could stop working, that the rescue timer will undo a failed check). | 2026-11-13 |
| 36 | function | reproduced | Settings layout | Passkey management (and the vault) sits under Account, not under Security, where a security setting belongs. Rethink how the settings are grouped. Decided in 10c §1: Settings → Security holds passkeys, the vault, sessions (list and settings), bootstrap networks and trusted proxies; Account keeps only the person. | 2026-11-13 |
| 38 | visual | reproduced | Settings → category list | Under the categories it still says "More categories arrive with the features they configure." Leftover from building it; every slice is done. (improve-ui) | 2026-11-14 |
| 39 | function | reproduced | Settings → Appearance → Comfort | "Reduce transparency" takes effect the moment it's switched but is only stored by the category's Save, while the theme cards above save on click. Leave without Save and the screen shows a setting that isn't saved. (improve-ui) | 2026-11-14 |
| 46 | later | reported | Code size | The Settings page renders its categories inline (785 lines). Split it into one component per category when 13.4 regroups the categories, keeping drafts per bug 91. (The firewall manager half was split in 13.1.) (code quality review) | 2026-11-14 |
| 48 | function | reproduced | Settings → General → Trusted proxies | Accepts `0.0.0.0/0`, `::/0` or any public range without a word. With that, every visitor counts as a proxy and the leftmost forwarded address wins (`clientip.Resolve`), so anyone can claim any IP: session IP binding, Hashi's rate limits and the recovery-link network check all go by it. Not an attack path on its own (only the admin can set it, with a passkey), but one wrong line removes every IP-based check. Refuse whole-internet and public ranges, or warn hard. (sharp-edges, fp-check: real hazard, not exploitable without the admin) | 2026-11-14 |
| 49 | later | reported | Settings → Security → Bootstrap networks | Accepts any range, including `0.0.0.0/0`, without a passkey, while the recovery link refuses with "works only from a private network". After setup the bootstrap login is off and a recovery link still needs a one-time console token (15 min), so this is only an extra layer; still, the setting should take private ranges only, and its text should say it now only governs recovery links. (sharp-edges, fp-check: defence in depth, not a vulnerability) | 2026-11-14 |
| 51 | later | reported | Spec 02 §2 | "Destructive operations need a passkey" never says which. Today deleting a hand-made monitor drops it and its whole history at once with no passkey (`monitor/views.go`), while DNS deletes become plans and rule sets can't be deleted while attached. Decide in the spec what counts, then make the code match. Also: "DNS prune needs a passkey" holds only through plan approval, which the spec should say. (spec-to-code-compliance) Proposal: a delete that cannot be undone and is not a plan (a check with its history, a script, an IP set, a rule set, a connection, an agent) asks for a passkey and names what is lost; deletes in DNS, Traefik and firewall stay plans approved as usual. Spec first (`docs(spec):`), then code. | 2026-11-14 |
| 59 | function | reproduced | Status → a check → Pause / Resume | Saves the whole form, including edits not yet saved (`save({ ...s, paused })` with the draft `s`). Codex changed Group without saving, pressed Pause, reloaded: the new group was stored. Pause should only send paused. (Codex F-020) | 2026-11-14 |
| 61 | visual | reproduced | Leftover development text | Like bug 38, more promises that are long kept: Add service says "Access rules arrive later; until then … lets requests through" (`services/+page.svelte:425`) though Access exists; a service's Monitoring tab says checks "arrive with monitoring (S7)" with no link, while Status runs its check (`services/[id]/+page.svelte:547`); Security → Challenge explains a rule action "Pass CAPTCHA" that the rule editor doesn't offer (`ChallengeSettings.svelte:179`, `rules.ts` says it "arrives with the abuse engine"). (Codex F-002, F-004, F-008) Correction: the server already supports `pass_captcha` (edge engine, S10); only the rule editor leaves it out (`rules.ts` offers none, deny, pass_authentication). Add it to the editor, then the Challenge text is true. | 2026-11-14 |
| 62 | visual | reproduced | Service → Rendered config (TCP/UDP) | The heading always says `10-hashi-http-services.yml`, also above a `tcp:` document; the title is hard-coded (`services/[id]/+page.svelte:555`). The design mock already switches to `20-hashi-stream-services.yml`. (Codex F-005) | 2026-11-14 |
| 65 | later | reported | Where a change is saved | The public page switches save on Save in Settings → Appearance but at once in App display, with nothing saying so (Codex F-025, same family as bug 39). A service's WAF tab has its own "Save WAF" under the header's "Save and publish", and nothing says which one saves the WAF draft (F-029). Decided in 10c §5: forms save with one Save (the WAF draft with the header's Save, no "Save WAF"); per-service public visibility is an instant switch, page-wide public options save with Settings → Public pages. | 2026-11-14 |
| 75 | function | reproduced | Security → any address link | Opens the design mock `/design/ip-sets/address` with made-up data (a sample country and AS, ban counter 0, "No events") instead of the real address page. `security/AddressLink.svelte` hard-codes the mock route; the IP sets copy of the component uses the real one. Used by Dashboard, Counters and Dynamic entries. (Codex R-002, checked in code) | 2026-11-15 |
| 76 | function | reported | Service save message | "1 plan(s) need your approval" after every service save, while the only waiting plan is the unrelated firewall plan from before. The save handler adds every plan the reconcile returns, including ones that were already waiting (`services_handlers.go`, `sync/engine.go` returns the existing plan when nothing changed). Count only plans this save created or changed. Also "plan(s)". (Codex R-003) | 2026-11-15 |
| 77 | function | reported | TCP/UDP service saved while the firewall plan waits | List and detail say "Published" / "Published on every Traefik host" although both hosts' plans still wait and nothing reached the hosts. Published is derived from enabled + valid only. (Codex R-007) | 2026-11-15 |
| 78 | function | reported | Service that is Off → Rendered config | Shows the full route as "What Hashi renders for this service on every Traefik host", but an Off service isn't rendered (the preview forces enabled, `render_services.go`). Say it's off and not deployed. (Codex R-004) | 2026-11-15 |
| 79 | function | reproduced | Public app dashboard header | "6 / 7 hosts online" counts app cards, not hosts: showing one more service makes it "7 / 8" (`monitor/views.go` adds one per card, and counts Degraded as online). (Codex R-005, checked in code) | 2026-11-15 |
| 80 | function | reproduced | Public status page with nothing public | Says "All services are working." above "Nothing is listed here yet." (zero down = all working, `PublicStatus.svelte`); "Updated" also comes from all checks, not only public ones. (Codex R-006) | 2026-11-15 |
| 81 | function | reported | Security → Counters, invalid search | After the error the old 213 rows, the selected row and "Reset counters of 1" stay active, so a reset can act on rows from an earlier search (`CountersView.svelte` returns before clearing). (Codex R-008) | 2026-11-15 |
| 82 | function | reported | Status → check detail, two tabs | Saving in a stale tab silently overwrites what another tab saved; the whole form is sent with no version check (`status/[id]/+page.svelte`, `monitor/views.go`). Codex lost a saved group this way, also via App display. (Codex R-012) | 2026-11-15 |
| 84 | function | reproduced | Settings → Internal agent DNS → Preview names | Previews the saved settings, not the draft on screen: the button sends no form data (`AgentDns.svelte`). (Codex R-019, checked in code) | 2026-11-15 |
| 85 | function | reported | Scripts → schedule preview | "Next" is shown in the browser's time zone without saying so, while the form says host time and the server computes UTC: 03:00 shows as 04:00. (Codex R-014) | 2026-11-15 |
| 86 | visual | reproduced | Status list and check detail | AdGuard and Pulse checks are grouped under "Other", have no source label, and the detail starts with ": its checks follow what it watches." `sourceText` in `lib/monitor.ts` lacks `adguard` and `pulse`. (Codex R-011, checked in code) | 2026-11-15 |
| 87 | visual | reported | Status → history strip | Always says "60 checks: 0 down, 0 degraded", also before the first check; the strip pads to 60 with unknown and counts those (`lib/monitor.ts`, `HistoryStrip.svelte`). (Codex R-009) | 2026-11-15 |
| 88 | visual | reported | Status → check detail → charts | 24h, 7d and 30d keep saying "Charts appear after the first full minute of checks" while 1h already has points, and 24h is the default, so a new check looks empty. (Codex R-010) | 2026-11-15 |
| 89 | later | reported | IP set → Resolved, large feeds | "Showing the first 200 of 1621 ranges." with no way to see more, although the API has a cursor and Items has "Show more". (Codex R-013) | 2026-11-15 |
| 93 | visual | reported | Every page: help text and eyebrows | Far too much text. Almost every panel and section has a description line, and most say what the page already shows or only matter in an edge case. Example: DNS → Records says records it doesn't own are shown but never changed outside import and prune, while the rows already say Owned / Unmanaged / Managed. It reads as walls of disconnected sentences and looks AI-made. Remove descriptions that repeat what's visible; keep a sentence only where it prevents a real mistake, and move edge-case explanations behind a help icon or into the place where the edge case happens. (owner) Includes the eyebrow review of old bug 28. Rules: 10c §4. | 2026-11-16 |
| 94 | visual | reported | Every page: too much at once | Pages show every option, setting and explanation at the same time. Use progressive disclosure: show what most visits need, put advanced options, rarely used settings and details behind a clear "More" / "Advanced" / expandable row / drawer, and show conditional fields only when their condition is on. Decide the rule once (see the UI rules to be written) and apply it everywhere. (owner) Rules: 10c §3. | 2026-11-16 |
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

### 36 · Settings grouping

The spec puts it there: 10b §3 lists "Account (passkeys, sessions, vault mode)". So the fix starts with a `docs(spec):` change.

What is where today:

- **Account:** Passkeys, Active sessions, Vault.
- **Security:** Admin sessions (session lifetime settings), Bootstrap networks.
- **General:** Domains and addresses, Times, Trusted proxies.

So sessions are split over two categories (the list under Account, their settings under Security), and passkeys, the
vault and trusted proxies, all security settings, sit elsewhere. Proposal for the fix: Security holds passkeys, the vault,
sessions (list and settings), bootstrap networks and trusted proxies; Account keeps only what is about the person.
