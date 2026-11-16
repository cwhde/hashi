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
| 17 | function | reported | Services → target | Giving a service a Pulse agent as its target takes a detour: first an IP set for the agent, then that set in the service, so every agent needs its own one-item set. Pick the agent directly in the service instead: address, agent or IP set, as AdGuard connections already allow (`target_agent_id`). Raised again by the owner 2026-11-16, no longer "later". | 2026-11-03 |
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
| 55 | later | reported | Web dependencies | `pnpm audit`: 7 advisories (3 moderate, 4 low) in SvelteKit, adapter-static, minimatch, deepmerge and cookie. None reach the running app: Hashi ships the SPA as static files served by Go, so SvelteKit's server paths (form actions, adapter-node, prerender headers) never run, and the rest are build tools. Still bump `@sveltejs/kit` to ≥ 2.70.4 and `@sveltejs/adapter-static` to ≥ 3.0.11 in the next dependency update. `govulncheck` (Go, call-graph based) found nothing. | 2026-11-14 |
| 56 | function | reproduced | IP sets → Add set / Add item → Host | "Host" is always greyed out with "Add a firewall host first.", although two hosts exist. Neither page passes the host list to `IpItemEditor` (`hosts` defaults to empty), so a Host item can never be added. (Codex F-009, checked in code) | 2026-11-14 |
| 57 | function | reproduced | IP set → Used by | Access rules never count as users of a set: only services, DNS, Traefik SSH, AdGuard and the firewall register (`app.go` `AddConsumers`), the rules module doesn't. Every set a rule uses says "Not used" / "Nothing uses this set yet" (seen on `all`, which a service's sign-in rule uses). Deleting such a set is still stopped by the database (`ON DELETE RESTRICT`), so the owner gets an error instead of the list of rules. (Codex F-010, cause found in code) | 2026-11-14 |
| 58 | function | reproduced | IP sets → address lookup → Firewall | The panel is a fixed sentence, "Verdicts appear here once a firewall host is registered.", shown even with two hosts registered; no verdict is ever computed (`ip-sets/address/+page.svelte`). (Codex F-011) | 2026-11-14 |
| 59 | function | reproduced | Status → a check → Pause / Resume | Saves the whole form, including edits not yet saved (`save({ ...s, paused })` with the draft `s`). Codex changed Group without saving, pressed Pause, reloaded: the new group was stored. Pause should only send paused. (Codex F-020) | 2026-11-14 |
| 60 | visual | reproduced | Browser tab title, 14 pages | Services, Service detail, IP sets, IP set detail, address lookup, Rule sets, Rule set detail, Firewall, Firewall host, Add firewall host, Traefik, Security, Pulse, Connections set no `<title>`. Opened fresh they show the bare URL; reached from another page they keep that page's title (tested: every one of them still said "Pending changes · Hashi"). (Codex F-007 found one page; checked live) | 2026-11-14 |
| 61 | visual | reproduced | Leftover development text | Like bug 38, more promises that are long kept: Add service says "Access rules arrive later; until then … lets requests through" (`services/+page.svelte:425`) though Access exists; a service's Monitoring tab says checks "arrive with monitoring (S7)" with no link, while Status runs its check (`services/[id]/+page.svelte:547`); Security → Challenge explains a rule action "Pass CAPTCHA" that the rule editor doesn't offer (`ChallengeSettings.svelte:179`, `rules.ts` says it "arrives with the abuse engine"). (Codex F-002, F-004, F-008) | 2026-11-14 |
| 62 | visual | reproduced | Service → Rendered config (TCP/UDP) | The heading always says `10-hashi-http-services.yml`, also above a `tcp:` document; the title is hard-coded (`services/[id]/+page.svelte:555`). The design mock already switches to `20-hashi-stream-services.yml`. (Codex F-005) | 2026-11-14 |
| 63 | later | reported | Services list and service Upstream | A service that follows an IP set shows the set's internal ID (`@<ULID>:<port>`) instead of its name, in the list and the detail summary; only the picker shows the set's name. The set's Used by lists "Service <name>" as plain text, not a link. (Codex F-026) | 2026-11-14 |
| 64 | later | reported | Missing links between related things | A check on Status has no link to its service (Codex F-012); per-connection "Show rewrites" opens the same unfiltered AdGuard tab for both instances (F-015); App display has no link, address or port for either public page it controls (F-013); the address lookup has no way to "Ban now", which lives only in the dynamic-blocked set (F-027); Pulse's DNS name field doesn't link to the suffix setting, while DNS → Internal agent names does (F-028); AdGuard rewrites and Internal agent names have no filter, Records has one (F-006). | 2026-11-14 |
| 65 | later | reported | Where a change is saved | The public page switches save on Save in Settings → Appearance but at once in App display, with nothing saying so (Codex F-025, same family as bug 39). A service's WAF tab has its own "Save WAF" under the header's "Save and publish", and nothing says which one saves the WAF draft (F-029). | 2026-11-14 |
| 66 | function | reproduced | IP sets → Add set with a bad first item | "Create set" stays enabled with `300.1.1.1`; it creates an empty set, leaves the drawer and says "its first item was refused. Add it again below." with no reason and the typed value gone. Check the item before creating the set. (Codex F-016) | 2026-11-14 |
| 67 | visual | reported | Errors that outlive their input | IP set Add item: after "10.0.0.0/33 is not a prefix", switching the type to DNS name keeps the old error (Codex F-017). Address lookup: clear a bad address and press Enter, the error stays and the intro text doesn't come back (F-018). | 2026-11-14 |
| 68 | function | reported | Add service → Upstream address | Pasting two lines (`192.0.2.10` and `2001:db8::10`) gives `192.0.2.102001:db8::10` with no warning; whether the server refuses it is untested. (Codex F-019) | 2026-11-14 |
| 69 | visual | reproduced | Firewall host page at 390 px | The tab row is wider than the screen: page 415 px in a 390 px view, "Old script" ends at 412 px, the whole page scrolls sideways. (Codex F-021, measured again) | 2026-11-14 |
| 70 | visual | reproduced | Pending changes badge on a phone | Below `sm` the words are `hidden`, so the link's only name is the number, read out as "1". Needs an `aria-label`. (Codex F-022, checked in `AppShell.svelte`) | 2026-11-14 |
| 71 | visual | reproduced | App display, while loading | Says "0 apps · cards link to the public address" and "Services with a public name appear here." for about half a second on every visit, then "7 apps" (the count is computed from a list that is still empty). Show loading instead. (Codex F-024) | 2026-11-14 |
| 72 | visual | reported | Filters with no match | Services shows "0 of 52" above empty column headers (Codex F-003); the audit log shows an empty table with only "Load older entries" under it, without saying whether older entries are searched too (F-030). DNS and Counters already say "No … matches". | 2026-11-14 |
| 73 | visual | reproduced | Browser tab icon | The admin app's favicon is still the SvelteKit starter logo (`lib/assets/favicon.svg`, used in `routes/+layout.svelte`). (found while checking Codex's work) | 2026-11-14 |
| 74 | function | reproduced | Pending changes, with an AdGuard removal waiting | The page goes blank: no pending cards, "No plan has finished yet", while the badge counts them and the API returns them (seen on prod, 2026-11-15: the waiting firewall plan and 20 recent runs are hidden). AdGuard plans aren't in the "not DNS" list in `sync_handlers.go` (`dnsItems`), so a delete item gets `before.values: null`, and `valueText` in `web/src/lib/sync.ts` calls `.join` on it and throws. `sync.ts` also has no `adguard` entry in `subsystems`. Fix both sides; one bad plan must not hide the rest. Right now it also blocks removing a leftover test rewrite on the home AdGuard. (Codex R-001, checked in code and live) | 2026-11-15 |
| 75 | function | reproduced | Security → any address link | Opens the design mock `/design/ip-sets/address` with made-up data (a sample country and AS, ban counter 0, "No events") instead of the real address page. `security/AddressLink.svelte` hard-codes the mock route; the IP sets copy of the component uses the real one. Used by Dashboard, Counters and Dynamic entries. (Codex R-002, checked in code) | 2026-11-15 |
| 76 | function | reported | Service save message | "1 plan(s) need your approval" after every service save, while the only waiting plan is the unrelated firewall plan from before. The save handler adds every plan the reconcile returns, including ones that were already waiting (`services_handlers.go`, `sync/engine.go` returns the existing plan when nothing changed). Count only plans this save created or changed. Also "plan(s)". (Codex R-003) | 2026-11-15 |
| 77 | function | reported | TCP/UDP service saved while the firewall plan waits | List and detail say "Published" / "Published on every Traefik host" although both hosts' plans still wait and nothing reached the hosts. Published is derived from enabled + valid only. (Codex R-007) | 2026-11-15 |
| 78 | function | reported | Service that is Off → Rendered config | Shows the full route as "What Hashi renders for this service on every Traefik host", but an Off service isn't rendered (the preview forces enabled, `render_services.go`). Say it's off and not deployed. (Codex R-004) | 2026-11-15 |
| 79 | function | reproduced | Public app dashboard header | "6 / 7 hosts online" counts app cards, not hosts: showing one more service makes it "7 / 8" (`monitor/views.go` adds one per card, and counts Degraded as online). (Codex R-005, checked in code) | 2026-11-15 |
| 80 | function | reproduced | Public status page with nothing public | Says "All services are working." above "Nothing is listed here yet." (zero down = all working, `PublicStatus.svelte`); "Updated" also comes from all checks, not only public ones. (Codex R-006) | 2026-11-15 |
| 81 | function | reported | Security → Counters, invalid search | After the error the old 213 rows, the selected row and "Reset counters of 1" stay active, so a reset can act on rows from an earlier search (`CountersView.svelte` returns before clearing). (Codex R-008) | 2026-11-15 |
| 82 | function | reported | Status → check detail, two tabs | Saving in a stale tab silently overwrites what another tab saved; the whole form is sent with no version check (`status/[id]/+page.svelte`, `monitor/views.go`). Codex lost a saved group this way, also via App display. (Codex R-012) | 2026-11-15 |
| 83 | function | reported | Single-choice button groups (Security period, IP set kind, Services state, Status range, Dashboard sort) | Pressing the selected choice again unselects it. Security then errors "range must be 1h, 24h, 7d or 30d", IP sets shows "0 of 15", Settings shows nothing selected but "Saved." Root: the shared `toggle-group` allows an empty value. (Codex R-016) | 2026-11-15 |
| 84 | function | reproduced | Settings → Internal agent DNS → Preview names | Previews the saved settings, not the draft on screen: the button sends no form data (`AgentDns.svelte`). (Codex R-019, checked in code) | 2026-11-15 |
| 85 | function | reported | Scripts → schedule preview | "Next" is shown in the browser's time zone without saying so, while the form says host time and the server computes UTC: 03:00 shows as 04:00. (Codex R-014) | 2026-11-15 |
| 86 | visual | reproduced | Status list and check detail | AdGuard and Pulse checks are grouped under "Other", have no source label, and the detail starts with ": its checks follow what it watches." `sourceText` in `lib/monitor.ts` lacks `adguard` and `pulse`. (Codex R-011, checked in code) | 2026-11-15 |
| 87 | visual | reported | Status → history strip | Always says "60 checks: 0 down, 0 degraded", also before the first check; the strip pads to 60 with unknown and counts those (`lib/monitor.ts`, `HistoryStrip.svelte`). (Codex R-009) | 2026-11-15 |
| 88 | visual | reported | Status → check detail → charts | 24h, 7d and 30d keep saying "Charts appear after the first full minute of checks" while 1h already has points, and 24h is the default, so a new check looks empty. (Codex R-010) | 2026-11-15 |
| 89 | later | reported | IP set → Resolved, large feeds | "Showing the first 200 of 1621 ranges." with no way to see more, although the API has a cursor and Items has "Show more". (Codex R-013) | 2026-11-15 |
| 90 | visual | reported | Connections → AdGuard form with HTTP | Certificate check, fingerprint and port 443 stay offered after switching to HTTP; the server then refuses a fingerprint. Hide or disable them for HTTP. (Codex R-015) | 2026-11-15 |
| 91 | visual | reported | Settings, switching categories | Some categories keep a draft (General, Account, Security, Monitoring, Firewall), others silently drop it (custom theme, Notifications, Pulse, Internal agent DNS). Same family as bug 40. (Codex R-017) | 2026-11-15 |
| 92 | visual | reported | Settings → Appearance → Reduce transparency | Header and dialog/sheet backdrops still blur: in the built stylesheet the `data-solid` override keeps only `-webkit-backdrop-filter`, which Chrome ignores. (Codex R-018) | 2026-11-15 |
| 93 | visual | reported | Every page: help text and eyebrows | Far too much text. Almost every panel and section has a description line, and most say what the page already shows or only matter in an edge case. Example: DNS → Records says records it doesn't own are shown but never changed outside import and prune, while the rows already say Owned / Unmanaged / Managed. It reads as walls of disconnected sentences and looks AI-made. Remove descriptions that repeat what's visible; keep a sentence only where it prevents a real mistake, and move edge-case explanations behind a help icon or into the place where the edge case happens. (owner) | 2026-11-16 |
| 94 | visual | reported | Every page: too much at once | Pages show every option, setting and explanation at the same time. Use progressive disclosure: show what most visits need, put advanced options, rarely used settings and details behind a clear "More" / "Advanced" / expandable row / drawer, and show conditional fields only when their condition is on. Decide the rule once (see the UI rules to be written) and apply it everywhere. (owner) | 2026-11-16 |
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
