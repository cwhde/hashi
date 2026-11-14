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

Review them together before the V1 cutover; visual and later entries wait until the app works.

| # | Kind | Status | Where | What | Reported |
| --- | --- | --- | --- | --- | --- |
| 1 | visual | reproduced | Sidebar | The items move when the bar expands on hover: aiming at DNS, the cursor ends up on Services once the bar opens. Items should keep their height. | 2026-10-31 |
| 3 | visual | reproduced | Several pages, e.g. a "Choose a set" selector | Items in a row aren't level: the selector sits at a different height from its neighbour. Seen in several sections; find them all when fixing. | 2026-10-31 |
| 4 | function | reproduced | Top bar search | Search results are generally not that good. | 2026-10-31 |
| 5 | later | reported | Browser tab, logo | No favicon yet; the current icon is a good placeholder but should be replaced. | 2026-10-31 |
| 7 | visual | reported | Delete confirmation dialog | The dialog has no border, unlike other UI items in every theme. | 2026-10-31 |
| 6 | visual | reproduced | IP sets → add an item | The options of the first field (type) wrap onto two rows, which breaks the layout. | 2026-10-31 |
| 8 | visual | reproduced | All on/off switches | In the on position the switch doesn't fully slide to the right and looks broken. | 2026-10-31 |
| 11 | later | seen | CI, `internal/firewall` | `TestIntegrationRescueRollback` hung in `Apply` on a `docker exec` until the 10 min test timeout (PR #55, run 1110, first attempt); the rerun passed, and so does a local run. Probably the backgrounded rescue timer keeping the exec's output open on that runner. Not reproduced. | 2026-11-02 |
| 12 | later | reported | Alerts | Group alerts: when almost all services go down at once (one host or the edge is down), send one grouped alert instead of one per service, and plan how to avoid the flood. | 2026-11-02 |
| 14 | function | reproduced | Pulse agent command line | `hashi-pulse --version` (and `version`) does not print the version: it starts a second agent beside the running one, whose heartbeat Hashi answers with 429. Seen on an edge container 2026-11-02; the extra process was stopped by hand. | 2026-11-02 |
| 15 | visual | reproduced | Service page header on a phone | At about 390 px wide the service name runs under the "Turn off" button in the header card (seen in a browser-test screenshot, 2026-11-02). |
| 16 | later | seen | CI, `internal/edge` | `TestIntegrationPocketID` panicked at its setup: Pocket ID v2.19.1 answered the OIDC client creation without `createdSecret` (main run 1131 after `77bded2`). The same code passed on the PR and three times locally. Not reproduced; maybe Pocket ID not fully started. | 2026-11-02 |
| 17 | later | reported | Services → target | Giving a service a Pulse agent as its target takes a detour: first an IP set for the agent, then that set in the service. Pick the agent directly in the service instead. | 2026-11-03 |
| 18 | later | reported | Service → WAF → exclusions | Typing bare rule numbers (920440, …) is unintuitive. Show each rule as a short readable name and offer a search by that name. The names should come from the rules themselves (every CRS rule carries a `msg` in the embedded rule files), not from a list we keep by hand. | 2026-11-03 |
| 20 | function | seen | Service behind the home server | The home server wasn't a host in Hashi (only the VPS is), so for a service whose target is at home Hashi can't tell which entry it sits behind and silently creates neither a public record nor an AdGuard rewrite. Explains why so few home services have rewrites. Add host now offers "behind a router" with its Traefik and entry host; left: the service page must say why nothing is published when no host matches. | 2026-11-03 |
| 21 | later | reported | Pulse agent on OpenWrt | Pulse should run on OpenWrt routers (and that kind of system in general: BusyBox, musl, procd instead of systemd/OpenRC), on arm64 and x86-64. The static binaries probably start already; install, service start (procd init script), self-update and address detection are untested there. Before 1.0. | 2026-11-03 |
| 22 | later | reported | Pulse agent page | Show the addresses an agent detected (per interface) and let the admin pick its main address, which services and DNS then use. Today the agent picks it and the page doesn't show the choice. | 2026-11-03 |
| 23 | function | reported | Firewall host behind a router | Its public address can only be typed in. The spec (05 §1.1) says it comes from the host's egress or from a Pulse agent, or is set by hand; the agent option was never built. A home address that changes then leaves the host records stale. Pick a Pulse agent as the source of the public address (and of the host's networks). | 2026-11-03 |
| 24 | function | reported | Connections page vs. the rest | We built a Connections page on purpose for every kind of connection, yet Traefik hosts, firewall hosts and Pulse agents are each added and configured on their own pages with their own forms. Redesign how things are set up and linked: add and configure all connections (reverse proxies, firewall hosts, Pulse agents, …) from the Connections page, and let services, hosts and sets reference them directly (see also 17, 22, 23). | 2026-11-03 |
| 26 | later | reported | Abuse engine → thresholds | Measure the thresholds (L7 rate, L4 meters, WAF count) against realistic clients, e.g. simulated Windows devices and browsers that open many connections and requests in the background, and tune the defaults so normal use never trips a challenge or a ban. | 2026-11-11 |
| 27 | later | reported | Code, everywhere | Check for implicit state machines (states spread over several booleans or strings) and, where it helps, turn them into one explicit enum. | 2026-11-11 |
| 28 | visual | reported | UI, all pages | Eyebrow optimisation: review the small labels above headings and cards; keep the ones that carry information, drop or rework the rest. | 2026-11-11 |
| 30 | function | reported | Public pages → assets | The asset system of 07 §9 (default icons, logos and backgrounds downloaded once to /data/assets, replaceable in Settings → Appearance) was never built in S7. Until it exists the Appearance settings for logo, icon and public page assets have nothing to change. | 2026-11-11 |
| 31 | function | reported | Overview page | The overview page from the design mock (widgets: pending changes, needs attention, service health, …) is not built, so the Appearance setting for widget order and visibility has nothing to change. | 2026-11-11 |
| 33 | later | seen | CI, e2e | The setup e2e hung at step 8: the page stayed at "Opening…" waiting for `/api/setup/https/start` until the 240 s test timeout, and the next test got no answer from Hashi either (CI run 1207, job 3130, on `04c4e25`). The rerun (job 3137) and local runs pass. Not reproduced; Hashi's log isn't kept by the e2e harness, so the next step is to keep it. | 2026-11-12 |
| 34 | function | seen | Firewall plan on the VPS | A feed added 129.73.0.0/16 to `block4`; the high-risk plan "block4 newly blocks 129.73.0.0/16" was approved during the S12 restore drill (07:28 UTC 2026-11-13), applied, and the check afterwards said the host can no longer reach NetBird's management server, so the rescue timer put the old table back at 07:31:20 (worked as designed). Why blocking that range breaks the NetBird check is not known yet; the same plan waits unapproved on prod. Reproduce on the dev target before touching it. | 2026-11-13 |
| 35 | function | reproduced | Changes → approve | The owner didn't know what the high-risk firewall plan would do and approved it; the approval dialog doesn't say plainly what is at stake (which addresses get blocked, what could stop working, that the rescue timer will undo a failed check). | 2026-11-13 |
| 36 | function | reproduced | Settings layout | Passkey management (and the vault) sits under Account, not under Security, where a security setting belongs. Rethink how the settings are grouped. | 2026-11-13 |
| 37 | function | reported | Settings → Vault → Add a passkey | "Add a passkey" only asks for a passkey that already exists (a sign-in request), so a new USB security key gets no passkey created on it. To use one, it has to be added as a sign-in passkey elsewhere first, which nobody would guess. The button should create a new passkey with PRF when needed. Also the sign-in "Add passkey": it first runs a re-confirmation (`withReauth`, a read limited to the passkeys Hashi knows) and only then the create, with nothing on screen saying so; sending the first prompt to the new USB key ends in "no stored passkeys for this website" and the create never comes (owner, Firefox + Bitwarden extension + macOS, 2026-11-13). Label both steps. Context: the Vaultwarden browser extension's passkey answered without PRF, so it can't unlock; the page should say that plainly. | 2026-11-13 |
| 9 | later | seen | Traefik install plan | One failed download of the Traefik release (network timeout) fails the whole plan; there is no retry, so an install during an edge replacement extends the outage until someone runs it again. | 2026-11-01 |
| 38 | visual | reproduced | Settings → category list | Under the categories it still says "More categories arrive with the features they configure." Leftover from building it; every slice is done. (improve-ui) | 2026-11-14 |
| 39 | function | reproduced | Settings → Appearance → Comfort | "Reduce transparency" takes effect the moment it's switched but is only stored by the category's Save, while the theme cards above save on click. Leave without Save and the screen shows a setting that isn't saved. (improve-ui) | 2026-11-14 |
| 40 | function | reported | Every editor | Nothing warns before leaving a page with unsaved edits (no navigation guard anywhere in the app): Settings, a service, a rule set, a script lose their edits silently. (web-design-guidelines) | 2026-11-14 |
| 41 | function | reproduced | Tabbed pages | Only DNS, Traefik and IP set pages keep the open tab in the address. Security, Activity, Pulse, Status, a firewall host and a service page don't: reload, back or a shared link always opens the first tab. (web-design-guidelines) | 2026-11-14 |
| 42 | visual | reproduced | Phone width | Tabs are 21 px tall on DNS, Traefik, Security, Status and Activity; the firewall page's "Remove" icons are 21 × 21 next to 19 px links. Below the 24 px minimum target, far below a comfortable 44 px. The address page has no heading when opened empty. (impeccable audit) | 2026-11-14 |
| 43 | later | reported | Forms | 20 of 186 inputs say what they hold (`autocomplete`), 1 turns spell-check off: IP, CIDR, domain and name fields get red squiggles and password-manager fill offers. (web-design-guidelines) | 2026-11-14 |
| 44 | later | reported | Numbers, browser chrome | No tabular figures anywhere, so counts in tables and tiles change width as they tick; no `theme-color`, so the phone browser bar ignores the theme. (web-design-guidelines) | 2026-11-14 |
| 45 | later | reported | Type sizes | About 1,000 hand-set pixel font sizes in 14 different values instead of a few named steps; 15 px and 24 px aren't in the spec's scale. Same root as bug 8: px and rem mixed on a 14 px base. (impeccable audit) | 2026-11-14 |
| 46 | later | reported | Code size | `internal/firewall/manager.go` is 1,363 lines with five jobs (connection, host records, policy, probing, sync); the Settings page renders seven categories inline (803 lines). Split along those seams. (code quality review) | 2026-11-14 |
| 47 | later | reported | Firewall renderer | The nft JSON is built from untyped maps (`obj`, `[]any`), so a misspelt key compiles and only fails on a real host. Typed structs for the few statements used would catch it at build time. (code quality review) | 2026-11-14 |
| 48 | function | reproduced | Settings → General → Trusted proxies | Accepts `0.0.0.0/0`, `::/0` or any public range without a word. With that, every visitor counts as a proxy and the leftmost forwarded address wins (`clientip.Resolve`), so anyone can claim any IP: session IP binding, Hashi's rate limits and the recovery-link network check all go by it. Not an attack path on its own (only the admin can set it, with a passkey), but one wrong line removes every IP-based check. Refuse whole-internet and public ranges, or warn hard. (sharp-edges, fp-check: real hazard, not exploitable without the admin) | 2026-11-14 |
| 49 | later | reported | Settings → Security → Bootstrap networks | Accepts any range, including `0.0.0.0/0`, without a passkey, while the recovery link refuses with "works only from a private network". After setup the bootstrap login is off and a recovery link still needs a one-time console token (15 min), so this is only an extra layer; still, the setting should take private ranges only, and its text should say it now only governs recovery links. (sharp-edges, fp-check: defence in depth, not a vulnerability) | 2026-11-14 |
| 50 | later | reported | Code, passkey PRF | `BeginPRF` uses its `salts` argument for two things: per-passkey salts when unlocking, an allow-list when adding an unlocker (bug 37's fix). Its comment still describes only the old use. A separate parameter would say what it is. (thermo-nuclear review of `bug-round`) | 2026-11-14 |
| 51 | later | reported | Spec 02 §2 | "Destructive operations need a passkey" never says which. Today deleting a hand-made monitor drops it and its whole history at once with no passkey (`monitor/views.go`), while DNS deletes become plans and rule sets can't be deleted while attached. Decide in the spec what counts, then make the code match. Also: "DNS prune needs a passkey" holds only through plan approval, which the spec should say. (spec-to-code-compliance) | 2026-11-14 |
| 52 | function | reported | Scripts → a run that times out | Shows no output at all, not even what the script printed before the limit: when the time runs out, `sshx.Run` returns an empty result (`internal/sshx/sshx.go`, the `ctx.Done` branch) and `scripts/run.go` shows that. Keep what was printed and add "stopped after N s". (semgrep, checked by hand) | 2026-11-14 |
| 53 | function | reported | Monitoring → TCP check with expected text | The check reads once, at most 4 KB (`monitor/checks.go`); a server whose greeting arrives in two packets counts as down although the text comes. Read until the text appears, the limit is hit or the timeout ends. (semgrep, checked by hand) | 2026-11-14 |
| 54 | later | reported | CI workflows | 25 steps use actions by moving tags (`actions/checkout@v4`, `setup-go@v5`, `cache@v4`, `pnpm/action-setup@v4`, …). The runners mount the production Docker host's socket and hold the registry and Pulse signing secrets, so a re-pointed tag would run there. Pin each to a commit SHA. (semgrep) | 2026-11-14 |

## Details

Checked 2026-11-14 on the production app (read-only: measured in the browser, nothing saved) and in the code. Pixel values
are at the app's 14 px base font.

### 1 · Sidebar items move when it opens

- **Reproduce:** move the mouse onto the closed sidebar and watch an item lower down, e.g. Activity.
- **Measured:** closed, Services sits at 62 px from the top and Activity at 465 px; open, at 85 px and 556 px. Each item
  drops 23 px for every section heading above it, so the lower the item, the further it jumps (Activity: 91 px).
- **Cause:** `AppShell.svelte`, `navList`: the section headings (Routing, Protect, Watch, …) are `sr-only` while the bar is
  closed (no height) and `h-6` when it is open. The headings need to keep their height when closed (empty or a thin rule).

### 3 · Controls in one row aren't level

- **Reproduce:** Firewall hosts page (`/firewall`), the "Choose a set" pickers with their "Add" button.
- **Measured:** the picker is 28 px tall (default size), the "Add" button 24.5 px (`size="sm"`); both are top-aligned, so
  the bottoms differ by 3.5 px. The same happens on a service page's route row: the fields are 28 px, "Remove route"
  24.5 px. On the DNS page a switch (18.4 px) sits next to "Add record" (24.5 px).
- **Cause:** default and `sm` sizes mixed in one row, top-aligned. Scanned every list page and two detail pages of each
  kind with a script; dialogs and sheets weren't opened, so check those when fixing.

### 4 · Search only finds pages

- **Spec:** 10b §2 promises addresses (to the address page), CIDRs, IP sets, rule sets, services, domains and pages.

- **Reproduce:** press ⌘K, type an IP address, a domain or a service name. Nothing is found.
- **What happens:** the box says "IP, CIDR, ASN, domain or page…", but in the live app it only lists the menu's page names
  (`AppShell.svelte`, `Command.Dialog`). The address and service results exist only in the design mock (`{#if !live}`),
  and there is no search endpoint in the API. So this is a missing feature presented as working, not weak results.

### 6 · IP item type options wrap

- **Reproduce:** open any IP set you can edit → "Add item".
- **Measured:** the side panel is 336 px wide even on a 1440 px screen, the seven type options don't fit, so "Other set"
  drops to a second row on its own. Same at phone width (panel 293 px).
- **Cause:** `ipsets/IpItemEditor.svelte`: a `flex-wrap` toggle group with seven options in a narrow sheet. Either a wider
  sheet, a two-row grid on purpose, or a select for the type.

### 7 · Dialogs look borderless

- **Cause (code, not measured):** every overlay (dialog, alert dialog, popover, select, dropdown) draws its edge as
  `ring-1 ring-foreground/10`, a faint halo, while Hashi's cards use `border` with the theme's border colour. In themes
  with a strong border colour the overlays look like they have none. Fix in the shared `ui/*-content.svelte` files.

### 8 · Switch knob doesn't reach the right

- **Measured:** every switch in Settings (Dashboard, Firewall, Notifications, Pulse): track 32 × 18.4 px, knob 14 px; on,
  the knob stops 5.4 px from the right edge, while off it sits 1 px from the left.
- **Cause:** `ui/switch/switch.svelte` sets the track in pixels (32 px) but the knob in rem (`size-4` = 1 rem). The app's
  base font is 14 px (`routes/layout.css`), so the knob is 14 px instead of 16 px and its slide (`100% - 2px`) falls short
  by 4 px. Other vendored shadcn parts that mix px and rem may have the same offset.

### 14 · `hashi-pulse --version` starts an agent

- **Reproduce (local):** `hashi-pulse version` prints the version and exits; `hashi-pulse --version` (or any other
  argument) is ignored and the agent starts. Without a config it stops with "url, agent_id and token are required"; on a
  host with a config it runs as a second agent beside the service, which is what was seen.
- **Cause:** `agents/pulse/main.go` only recognises the exact word `version`. Unknown arguments should be refused with a
  usage line; `--version` and `-v` should print the version.

### 15 · Service header on a phone

- **Now (390 px wide, all 52 services checked):** the title no longer runs under the buttons, but it's squeezed into a
  narrow column (about 63 px) next to "Turn off", "Delete" and "Save and publish", so even a two-word name breaks onto two
  lines and the name overflows its box on every service. The upstream address in the facts below breaks mid-number
  ("…:300" / "7").
- **Cause:** `routes/(app)/services/[id]/+page.svelte`: title and button group share a `flex-wrap` row; the buttons don't
  move below the title on narrow screens. Addresses need `break-all` or a horizontal scroll, not a mid-port break.

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
