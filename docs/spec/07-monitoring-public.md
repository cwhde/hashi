# 07 — Monitoring, notifications, public status page, public app dashboard

Hashi has built-in, Gatus-inspired monitoring. It replaces V1's generated Gatus config; no external monitor is required.

## 1. Endpoints

Sources: services, manual DNS entries with monitoring enabled (display name required), Linux firewall hosts, Traefik
connections, AdGuard connections, Hashi itself, Pulse agents (push health), user-created monitors.

Fields: name, group, source link, enabled/paused, interval (default 60 s; setting), timeout, checks, required-check set,
latency thresholds (degraded), allowed HTTP codes (default 200–399), public visibility, notification route override.

## 2. Check types

HTTP, HTTPS, H2C (where practical), TCP connect, UDP (only with a configured probe payload and expected response —
"no response" is never treated as healthy), DNS (query + expected answer), ICMP (only if the
container has `CAP_NET_RAW`; otherwise disabled with an explanation), TLS certificate expiry (warn at 21 d, critical at 7 d),
Pulse push health (heartbeat freshness).

Auto-detection: explicit service protocol first; for DNS entries, probe common ports in a safe fixed order (443, 80,
then the entry's configured port) once at creation, then use the chosen check; per-endpoint override; no scanning.

HTTP checks may target the public URL (through DNS + Traefik) and/or the internal upstream; both together distinguish
"service down" from "edge down" (degraded when only one path is healthy).

## 3. States

Up (all required checks pass) · Degraded (some fail, latency over threshold, or only some paths healthy) · Down (required
checks fail) · Paused · Unknown (not enough data). Transitions use thresholds (default: 3 failures → down, 2 successes → up).

## 4. Storage

`monitor_endpoints`, `monitor_checks`, `monitor_samples` (raw), `monitor_rollups` (1 m / 5 m / 1 h; count, ok count, min/
max/avg/p95 latency), `monitor_events` (state changes, incidents). Retention per 01-platform.md §4.3. Rollups only from
closed buckets. History strips keep per-sample timestamps.

## 5. Admin UI

Status list: compact row/card per endpoint, last-60-minutes strip, state, last check, response time, group; search; group
by host / firewall host / status / service type; sort by name / state / latency / uptime / last event.

Detail: current status, last check, response min/max/avg, uptime (1 h, 24 h, 7 d, 30 d), latency chart (1 h, 24 h, 7 d,
30 d), incident/event timeline, endpoint settings.

## 6. Notifications

Providers: SMTP (TLS/STARTTLS, auth, from/to), Telegram bot, Discord bot. Secrets in vault.

Easy setup: Telegram — after token entry, user messages the bot, Hashi polls `getUpdates` to discover chat/channel;
Discord — short pairing mode connects to the gateway and waits for a DM or mention to capture channel/user ID; manual IDs
always possible; SMTP — send test email.

Routing: global default route, per-endpoint override, severity thresholds, cooldowns, recovery notifications; pending
high-risk sync plans and vault-lock events can also notify. Deliveries recorded in `notification_deliveries` with retry/backoff.

The cooldown is per check: a second alert for the same check within it waits for its end and goes only if the check is
still in alarm. Alerts are grouped: a new alert waits a short window (route setting, default 30 s, 0 = off; checks run every 60 s, so one outage's alerts arrive tens of seconds apart), each further
alert for the same provider restarts it, and when it ends the waiting alerts go as one message listing them. No alert
waits longer than 2 minutes for grouping. Recoveries are not delayed. When a host's own check (Traefik host, firewall host, Pulse
agent) is in the group, the message leads with it and names it in the subject ("edge-a and 12 other checks are down").

Providers can be edited: name and settings, with the secret kept unless a new one is given. Setup lives under Settings →
Notifications; the Status page shows what was sent.

Message content: state with its colour, service name, a plain reason, time, uptime over 24 h, 7 d and 30 d, a link to the
admin page. Mail is multipart (plain text + HTML in the saved theme's colours); Telegram uses its HTML style. Messages pass
through third parties, so they never carry check targets, internal addresses or raw errors; those stay in Hashi's event history.

## 7. Public pages: one layout for both

Decided with the owner 2026-11-22 from the rough mocks of 2026-11-21. Both public pages read one layout: an ordered list
of groups, each holding an ordered list of entries. Each page shows the entries switched on for it, in the layout's
order, and skips the rest. Nothing on a public page knows whether the visitor is signed in to Hashi: no admin
detection, no edit links.

**Entries.** Every service (except Hashi's own system ones), every DNS entry that is on the dashboard or monitored
(the manual entries of 03 §7), and every endpoint that belongs to neither (user monitors, hosts, agents, Hashi itself)
is one entry, kept in the layout automatically by the monitor's source sync: a new one joins the end of "Other", a
removed one leaves. An entry has two switches:

| Switch | Possible when | New entries |
| --- | --- | --- |
| Apps | it has a web address (web service, DNS entry with one) | on for services, off for the rest |
| Status | it has a check | off |

A switch that is not possible is shown disabled with its reason ("No web address", "No check"). The entry's display name
is the service's or DNS entry's name, else the endpoint's.

**Groups.** Named by the owner, ordered by the owner. "Other" holds entries without a group, has no name to edit and is
always last; an empty "Other" is not shown. A group is deleted only when empty. The admin Status page groups by the same
groups in the same order (and never hides anything). The group is the endpoint's group from before: the endpoint
settings' Group field edits the entry's place in the layout.

**Storage.** `public_groups` (id, name unique, position) and `public_entries` (id, subject kind `service` | `dns` |
`endpoint` + subject id unique, group id or null for "Other", position), one `version` for the whole layout. The layout
holds order and groups only. The two switches stay where they already live and other pages already write them (one
truth, nothing to keep in step; changed 2026-11-22 while building): Apps is the service's `dashboard_visible` or the DNS
entry's `dashboard`, Status is the entry's endpoint's `public`. Turning Apps off on a DNS entry that is not monitored
takes it out of the layout; the DNS page brings it back. An entry's group is written to its endpoint's `group_name` in
the same transaction ("Other" for none), so notifications and older readers see the same group.
Migration: groups from the endpoints' group names in the old category order (then by name), entries in each group by
name (an entry without an endpoint in "Services" or "DNS entries", as the old apps page grouped them); the
hidden-categories and categories-off settings go: entries of a hidden category get Status off, and both pages always
show groups (2026-11-22).

**API.** `GET /api/public-layout` returns groups, entries (with name, kind, icon, what is possible) and `version`.
`PUT /api/public-layout` takes the whole order (groups with entry ids) and switch changes with `version`; a newer stored
version refuses the save (409) and the editor reloads. Audit: one event per save, naming what moved or changed.

### 7.1 Status page (port 8082, root path)

Enabled by default (setting). Only entries with Status on; never internal hostnames, IPs, ports, protocols, status codes,
error text or admin links.

- **Banner**: "N services are down." / slow / everything works, with "Updated" from public checks only (as since 13.5).
- **Group tiles**: one per group shown, in layout order: the group's name, a state dot (worst of its entries), "x of n
  up". A click scrolls to the group and opens it. Hidden when only one group is shown.
- **Group panels**: each group in its own panel that folds. A group with an entry down or slow starts open; the others
  start as the visitor left them (kept in the browser), open the first time.
- **Rows** (dense): icon (the app icon of §9.1, else the letter; always shown), name, history bar, uptime for the
  bar's span, state badge. Bars: 90 days, one per day, by default; a 90 days / Last hour switch above the groups shows
  the last 60 minutes, one per minute, instead. No caption explains the bars; the details are on hover.
- **Bar details**: pointing at a bar (or tapping it, or arrow keys on the focused bar) raises that bar, dims the rest
  and shows a bubble with what happened then. A day: the date, then each downtime with its times and length ("Down
  19:55–19:56 (1 min)"), each slow spell ("Slow for 20 min, 14:10–14:30"), else "No downtime", and that day's uptime;
  today says "Down since 13:36" while it lasts. A minute: the time, Up with its response time, Slow with it, Down, or
  No data. The strip stays one drawing per state (13.11); the raised bar is one extra element.
- The row opens the service page.
- Failed refresh shows a visible "data may be stale since …" banner rather than silently showing empty or stale data.
- Phone: tiles in two columns, rows stack name over bar, nothing wider than the screen.
- `GET /api/public/status` (cached 15 s): groups and entries in order, state, 60 one-minute and 90 one-day samples,
  uptime 24 h / 90 d. Days come from the 1 h rollups (01 §4.3 keeps them 2 years); a day without data is "no data".
  Each day and minute carries what its bubble needs: the downtimes and slow spells in it (start, end), the uptime, and
  for a minute the response time. Spells come from the state changes already stored (§4); nothing new is recorded.

### 7.2 Service page (status listener, `/s/<slug>`)

One page per entry with Status on, with an address to share. The slug is the service's, else made from the name; an
entry switched off answers 404.

- Head: icon, name, group, state badge, and one **Badges** button (§7.3).
- Tiles: Now ("Up for 14 h", "Down for 6 min"), uptime 24 h, 7 d, 30 d, average response 24 h.
- 90-day bar with its dates and the bar details of §7.1.
- Three charts side by side (stacked on a phone), one 24 h / 7 d / 30 d switch for all: **Response time** (average),
  **Slowest responses** (p95), **Availability**. Pointing at a chart shows that moment's time and value beside the
  pointer, as the admin endpoint page's charts do (`LineChart` with labels).
- Recent downtimes: the last 10 within 90 days, start, end and length; none: "No downtime in the last 90 days."
- Back link to the status page. `GET /api/public/status/<slug>` (cached 15 s), the same public fields only.

### 7.3 Badges

The service page has one Badges button; it opens a small panel with every badge this entry has, drawn as it will
look, and a Markdown / Link switch; a click on a badge copies it and says "Copied". SVG images for READMEs and other
pages, served by the status listener at `/badge/<slug>/<kind>.svg`, cached 60 s,
`Content-Security-Policy: default-src 'none'; style-src 'unsafe-inline'`, `nosniff`. Kinds: `status` (up, slow, down),
`uptime-24h`, `uptime-7d`, `uptime-30d`, `response-24h`, `certificate` (days left; only when the entry has a TLS check).
Look: 20 px high, 4 px corners, a dark label (#3d444d) and the value on its colour, white bold text with a faint
shadow, a light top sheen and a thin edge, so it reads on light and dark READMEs; the status badge's label is the
entry's name and its value has a dot. Colours are fixed in the file (not theme tokens), each at least 4.5:1 with white:
good #1a7f37, warning #9a6700, bad #cf222e, unknown #6e7781. Value colour: status from the state; uptime good from
99.5 %, warning from 98 %; response good under 300 ms, warning under 1 s; certificate good over 14 days, warning over
7. Text widths from a fixed width table for 11 px Helvetica/Arial; no fonts or images loaded from anywhere. The panel
lists each badge once. An entry with Status
off or an unknown kind answers 404.

## 8. Public app dashboard (port 8081, root path)

Launcher of the entries with Apps on, enabled by default (setting), no admin controls.

- Groups as headings in layout order; cards in layout order inside each. Sorting by Name or Status drops the
  headings and shows one list.
- Each card: app icon (§9.1), name, the public address; links to it. No up/down counts in the header (bug 147).
- Search field always shown (bug 146); sort selector: Your order (default), Name, Status.
- `GET /api/public/apps` (cached 15 s).

### 8.1 Public pages editor (admin `/public-pages`, renamed from App display)

The page holds the layout only. A gear in the header opens Settings → Public pages (10c §1.1, §6), which holds the
page-wide options: each page on or off, where it is published (bug 64), the apps page's default sort, page titles and
the asset slots of §9. No tabs, no description, no help sentences.

- **Outline** (left): one 28 px line per entry, about 30 on a 1080 px screen. Rows: grip, checkbox, icon, name, kind,
  the two switches. Group headers: grip, name (click to rename), count, Sort A–Z, delete when empty. New group at the
  end. "Other" has no grip and stays last.
- **Preview** (right): the public page as visitors see it, with an Apps page / Status page switch above it. A click on
  a card or row finds the entry in the outline.
- **Dragging is live**, as on a phone's home screen: the dragged row, card or group lifts and follows the pointer, and
  the others slide out of its way while it moves, not only when it is let go. It works the same in the outline and in
  the preview, and the other side follows at once. Where it lands is decided from each item's place in the layout,
  not where it is drawn while sliding, so a still pointer never makes items swap back and forth. Mouse: drag anywhere
  on a row or card (not on its switches); groups by their grip. Touch: the grip drags at once; elsewhere hold still
  for a moment first, so a swipe still scrolls. Several checked rows drag together.
- **No Move menu.** Keyboard: Alt+Up/Down on a focused grip moves the entry one place, across into the next group at
  a group's ends. Every move is announced to screen readers.
- **Selection bar** for checked rows: Move to (group), Show or hide (apps page / status page), Clear.
- Every change saves at once (one PUT per drag, at release); a bar offers Undo for the last change ("Immich moved",
  "Immich moved to Home", "Group Games moved").
- Phone: the outline only; a Preview button opens the page in a sheet.

## 9. Assets

Four slots for the public pages: icon (favicon and touch icon), logo for dark themes, logo for light themes, background.
Each slot is empty, downloaded or uploaded; Settings → Public pages sets them.

- Asset source: a base URL (setting, empty by default; the repo is public, so no host is built in). Hashi reads the
  source's folder listing (`/icon/<set>/`, `/logo/<2k|4k>/`, `/background/<2k|4k>/`) so the owner picks a file, then
  downloads it once into `/data/assets`. Visitors' browsers never load from the source.
- Upload: png, webp, svg or ico, at most 5 MB (background) or 1 MB (others). SVG is served with
  `Content-Security-Policy: default-src 'none'; style-src 'unsafe-inline'` and `nosniff`.
- Served at `/assets/<slot>` on the admin and both public listeners, with an ETag; an empty slot answers 404.
- Empty icon → Hashi's 橋 icon; empty logo → the page title as text. Background is off by default and, when on, sits
  behind a scrim that keeps text at its normal contrast (10-ui §2). Admin screens keep the theme canvas and the 橋 icon.

### 9.1 App icons (bug 149)

Each dashboard app's icon is, in order: (1) custom, uploaded or picked from the selfh.st icon collection
(`https://cdn.jsdelivr.net/gh/selfhst/icons@main`: `index.json`, files at `/{svg,png,webp}/<reference>[-light].<ext>`;
the index is cached 24 h, a search returns at most 30, ranked exact, prefix, contains, tag; the `-light` file is used on
dark themes when it exists; the picker credits "Icons by selfh.st, CC BY 4.0"); (2) automatic, fetched by Hashi from
the app (manifest icons, `apple-touch-icon`, sized `icon` links, `/favicon.ico`, largest that fits first); when the
public address answers 401/403, redirects to another host or names no icon, Hashi asks the upstream with the app's
Host header instead; refreshed daily, stored in `/data/assets/apps/`; (3) the name's first letter. Visitors' browsers
load app icons only from Hashi.

## 10. Tests that must exist

Check executors against local test servers (HTTP/H2C/HTTPS/TCP/UDP/DNS); state machine thresholds; rollup correctness
with closed-bucket rule; public APIs expose only selected fields (schema test); notification routing + cooldown;
Telegram/Discord/SMTP against recorded contracts or local fakes. Public layout: every service, DNS entry and endpoint is
in it exactly once; a stale version is refused; switches that are not possible are refused; both public APIs keep the
layout order and leave out switched-off entries; the service page and badges answer 404 for an entry with Status off; the
migration keeps the old category order, visibility and groups.
