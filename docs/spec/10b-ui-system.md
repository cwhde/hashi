# 10b — UI system (output of slice S0b)

The rules every page follows. The clickable mocks for every page live in the SPA under `/design` (index at `/design`);
feature slices build the real page at the same path without `/design` and replace the mock data with the generated API
client. Where this file and a mock disagree, this file wins and the mock gets fixed. Services, Firewall, Security, IP sets,
rule sets and the address page follow the security model (ADR 0002); their mock data lives in `web/src/lib/design/security.ts`
and the shared security components in `web/src/lib/components/hashi/security/`. Intent and palettes: [10-ui.md](10-ui.md).

## 1. Tokens

Two layers in `web/src/routes/layout.css`:

1. **Palette roles** per theme (`--h-canvas`, `--h-shell`, `--h-surface`, `--h-surface-2`, `--h-elevated`, `--h-border`,
   `--h-text`, `--h-muted`, `--h-accent`, `--h-accent-text`, `--h-accent-hover`, `--h-focus`), set on `[data-theme=…]`.
   Values are exactly 10-ui.md §2. A custom theme only provides these.
2. **Semantic names** derived once from layer 1 (shadcn names `background`, `card`, `popover`, `primary`, `muted`, `border`,
   `ring`, `sidebar`… plus Hashi's `canvas`, `shell`, `surface`, `surface-2`, `elevated`).

Status colours are fixed across themes: `ok #22C55E`, `warn #F59E0B`, `bad #EF4444`, `info #3B82F6`, `idle` (muted/surface
mix). Text shades (`*-text`) are lighter on dark canvases and darker on Warm Linen. Status is never shown by colour alone:
every dot or strip has a label or an accessible name.

Components use semantic Tailwind classes only (`bg-card`, `text-muted-foreground`, `bg-ok/10`, …), never hex values. The only
exception is the custom-theme editor, which previews colours the user typed.

`data-theme` + `data-mode` (`dark`/`light`) live on `<html>`; an inline script in `app.html` applies the stored theme before
first paint. `dark:` variants follow `data-mode`, not the OS.

| | Value |
| --- | --- |
| Type | Instrument Sans (UI, bundled), Geist Mono (literal values only: IPs, records, hashes, paths, commands) |
| Base size | Root 14 px. Type scale (tokens in `layout.css`, pixels): `2xs` 11.5 (tags, keyboard hints) · `xs` 12 · `sm` 12.5 (secondary) · `md` 13 (tables, forms) · `base` 14 · `lg` 15 (section headings) · `title` 22 (page and setup titles, tile numbers). No other font sizes |
| Radius | controls 10 px (`rounded-[10px]`/`rounded-lg`), panels 16 px (`rounded-2xl`), login/setup cards 24 px (`rounded-3xl`), pills only for status tags |
| Spacing | 4 px grid; page padding 16 px mobile / 32 px desktop; panel padding 16 px; gaps between panels 16–20 px |
| Rows | table and list rows 40–44 px; navigation rows 36 px; top bar 52 px |
| Width | content max 1480 px; prose max 70 ch |

## 2. Shell and navigation

- Left navigation: 64 px icon rail; expands to 240 px on hover or keyboard focus (overlaying content), or stays open when
  pinned (preference stored per browser). Below 768 px it becomes a drawer opened from the top bar.
- Groups (sentence case, no caps labels): **Overview** · **Routing**: Services, DNS, Traefik, Firewall hosts, Pulse ·
  **Protect**: Security, IP sets, Rule sets · **Watch**: Status, App display · **Operate**: Scripts, Activity ·
  **Configure**: Connections, Settings.
- Top bar (sticky): page title · search (⌘K: one search for everything, keyboard only: an address or CIDR → address page and
  the IP set items holding it; every stored object (services, DNS records, AdGuard rewrites, IP sets, rule sets, firewall
  and Traefik hosts, agents, connections, scripts, checks, plans, activity); every page, every tab (an object's tab by
  object and tab name, "gitea waf"); every setting by its label, opening Settings scrolled to it. Each result opens the
  exact place (`?find=`, `?tab=`). Pages, tabs and settings are matched in the browser, objects by `GET /api/search`) · **pending changes** count (always visible,
  amber when anything waits) · vault state · theme · account (session expiry and bound IP).
- Vault locked: an amber banner under the top bar on every page, with Unlock.

## 3. Page inventory

| Path | Page | Contents |
| --- | --- | --- |
| `/overview` | Overview | Pending high-risk plan first, needs-attention list (incl. unresolved targets, stale feeds, host link down), service health strips, edge hosts, sync state, security 24 h, certificates, recent audit. Widgets reorderable in Settings → Appearance |
| `/changes` | Pending changes | Every waiting plan with exact before/after per item, risk per row, passkey approval; recent runs |
| `/services` | Services | Filterable dense table (service, kind, target, access summary, 60-min strip, status, row menu); add-service drawer |
| `/services/{id}` | Service | Summary strip + tabs: Routing (domain, upstream address set, advanced routes), Access (root rule set editor, attached rule sets, default fallback, flattened list with Stage A on top, "try a request"), WAF (mode per path, exclusions), Monitoring, Rendered config. L4 services: Routing, Monitoring, Rendered config |
| `/dns` | DNS | Tabs: Records (inline row editor, show generated, protected NS/SOA), Import (leave/import/remove per RRSet), AdGuard rewrites per instance, Internal agent names |
| `/traefik` | Traefik | Tabs: Hosts (version, update available, generated files and drift), Your middlewares (YAML editor + found names), Certificates |
| `/firewall` | Firewall hosts | Global policy panel (IP sets bound to allow and to block, folded range counts, carve-out warnings); host panels (mode, subnets, ports, NetBird, host link, last apply, strip); external-NAT hosts show router port-forward instructions |
| `/firewall/{id}` | Firewall host | Rescue path, NetBird, host link state; tabs: Ports (with conflicts), Host access (IP sets), Kernel sets (sizes, staging now, meter thresholds), Live ruleset (all tables, Hashi's highlighted), Generated (rendered nft JSON) |
| `/pulse` | Pulse | Agents table (main/public IP, reachability, freshness); each name opens the agent |
| `/pulse/[id]` | Pulse agent | Addresses per interface and the main-address choice, its own settings (DNS name, reach port, trust), what follows it |
| `/status` | Status | Endpoint cards grouped by group or status with history strips; Notifications tab (recent deliveries, where alerts go, link to their setup) |
| `/status/{id}` | Endpoint | Summary metrics + strip, range switch, three equal charts, events |
| `/app-display` | App display | What the public dashboard (8081) and status page (8082) show, order and visibility |
| `/security` | Security | Tabs: Dashboard (allowed/challenged/banned/WAF tiles with sparklines, L4 meter trips per host, top addresses, countries, targeted services, challenge → ban funnel, events, stale feeds/GeoIP), Counters (every address with a ban or challenge counter; filters: banned now, challenged now, any active, all; minimum counts, source, country/ASN, time; search by address/CIDR; edit, reset, bulk reset), Challenge (Cap), Sign-in providers, Engine (thresholds, ban function with curve, GeoIP) |
| `/ip-sets` | IP sets | Every set in one table: name, kind (yours / system / feed / dynamic) with lock marker, size (items, resolved v4/v6), used by, status; search across all sets by address; add set / add feed |
| `/ip-sets/{id}` | IP set | Summary strip (kind, resolved size, used by, status); tabs: Items (search, add/edit/remove with the shared item editor, note, expiry; dynamic sets: entries with source, expiry, failures, both counters, remove / add with duration / edit counters), Resolved (compiled prefixes with origin path), Used by, Feed (feed sets: source, format, runs), History |
| `/ip-sets/address` | Address | Member of (sets with nesting path), firewall verdict per host with deciding prefix, dynamic state, ban and challenge counters with edit/reset, "try a request" trace for a service + path, timeline, actions |
| `/rule-sets` | Rule sets | `global` first, then reusable sets: entries count, used by, warnings |
| `/rule-sets/{id}` | Rule set | Ordered entries (drag), item editor (IP set picker, condition, action, auth options), attach a rule set, used by, warnings |
| `/scripts` | Scripts | Script list with run history ticks; editor, schedule, timeout, env, targets; runs with output |
| `/activity` | Activity | Tabs: Audit log, Jobs (last/next run, result), Live log |
| `/connections` | Connections | All external systems grouped by type, health, last check, required marker; adds and edits each (Traefik and firewall host SSH, Pulse agents with token and install, AdGuard); `?add=<kind>` opens its drawer for feature pages' add buttons |
| `/settings` | Settings | Category list; Appearance (themes, custom theme editor with contrast guard, widgets), Account (passkeys, sessions, vault mode), General, Notifications (providers with add/edit, default route), and every category in 09 §2 |
| `/states` | (reference) | Vault locked, empty, loading, error, confirmations |

Shared security components (05a §7): IP set picker (filters to address sets in target positions), IP item editor (type
switch), set badge (kind + lock), address link, rule item editor, flattened rule list. Every page that takes an address or
a rule uses them.
| `/login` | Sign in | Passkey only; recovery hint |
| `/setup` | Setup | 11 resumable steps (02 §1); host-key pinning shown at step 5 |
| public `:8081/` | App dashboard | Grouped launcher cards with state, online counts, collapsed search |
| public `:8082/` | Status page | Overall banner, grouped strips, staleness note; no hosts, IPs or errors |
| edge `/api/edge-challenge/start` | Visitor challenge | Minimal page, Cap widget, the domain being accessed |

## 4. Component rules

- **Panel** (`Panel.svelte`) is the only container: 1 px border, `bg-card`, 16 px radius, no shadow, optional header row
  (title, actions right; a muted line only as 10c §4 allows). No card per number; numbers sit inside the panel that explains them.
  The header is one line at a fixed height, a muted line sits beside the title, so cards side by side start their
  first rows at the same height; fields in neighbouring cards use the same spacing and every field has a visible label.
  Three or more cards of different heights side by side go into `CardColumns`: each column stacks on its own, so a
  short card never leaves a hole beside a tall one (bug 158). Equal cards in a list (hosts, endpoints) keep the grid.
- **Page header**: 22 px title, actions right; a description only as 10c §4 allows. Never a hero.
- Where things live, progressive disclosure, help text and save behaviour: [10c-ui-rules.md](10c-ui-rules.md), which wins
  over this file where they disagree.
- **Tables** for sets that are compared or scanned (services, records, IP sets, agents, audit). **Cards** only where side-by-side
  comparison of state matters (status endpoints, app launcher, firewall hosts).
- **Inline editing** for short records (DNS): editor opens under the row, full width, Save/Cancel/Delete in one row,
  secondary fields behind a disclosure. **Drawers** (right `Sheet`) for larger creates. **Tabs** split a detail page into one
  tool at a time, below a summary strip.
- **One primary button per area** (accent fill). Everything else outline/ghost. Destructive actions are text-red ghost buttons
  and always confirm, naming what will be planned.
- **History strip** (`HistoryStrip.svelte`): one bar per sample, oldest left, time labels at both ends, accessible summary.
- **Charts**: thin line, sparse dashed grid, current value top right, three equal charts per row on detail pages (uPlot in
  real pages). Sparklines: same, no axes.
- **Change preview** (`PlanReview.svelte`) is the signature element: every write is shown as verb · subsystem/target ·
  object · before (struck) · after, risk marked per row, high-risk approval needs a passkey. What is shown is what is applied.
- **Tags** (`Tag.svelte`): quiet by default; tone only for meaning (ok/warn/bad/info, accent for "managed by Hashi").
- **Mono** only for literal values. Labels, headings and buttons are never monospace.
- **States**: skeletons that keep the final layout (no page spinners); errors say what happened, that nothing changed, and
  how to fix it; empty states say what the thing is and offer the one action.

## 5. Writing

Sentence case everywhere, no all-caps labels. Buttons say what happens ("Save and review plan", "Approve and apply", "Plan
deletion"); the same verb carries through the flow. Remote changes show "Applying…" until the API confirms, never "done"
before. Errors never apologise and never hide the cause.
Counts and units are words: "1 change", "3 changes", never "change(s)"; "seconds", never "(s)" or "180 s". No exclamation
marks. Generated text (plan results, job summaries) follows the same rules at its source.

## 6. Accessibility and responsiveness

WCAG 2.2 AA: body text ≥ 7:1 and muted text ≥ 4.5:1 on surfaces in all built-in themes (tested in `contrast.spec.ts`);
the custom-theme editor refuses to save below 4.5:1. Visible focus ring (`--h-focus`) on every interactive element; full
keyboard use including ⌘K and the navigation rail (expands on focus). `prefers-reduced-motion` removes transitions.
Below 768 px: navigation drawer, tables drop secondary columns (target, access, strip) but always keep identity and status;
detail panels stack.
