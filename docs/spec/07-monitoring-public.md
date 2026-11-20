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
still in alarm. Alerts are grouped: a new alert waits a short window (route setting, default 15 s, 0 = off), each further
alert for the same provider restarts it, and when it ends the waiting alerts go as one message listing them. No alert
waits longer than 2 minutes for grouping. Recoveries are not delayed. When a host's own check (Traefik host, firewall host, Pulse
agent) is in the group, the message leads with it and names it in the subject ("edge-a and 12 other checks are down").

Providers can be edited: name and settings, with the secret kept unless a new one is given. Setup lives under Settings →
Notifications; the Status page shows what was sent.

Message content: state with its colour, service name, a plain reason, time, uptime over 24 h, 7 d and 30 d, a link to the
admin page. Mail is multipart (plain text + HTML in the saved theme's colours); Telegram uses its HTML style. Messages pass
through third parties, so they never carry check targets, internal addresses or raw errors; those stay in Hashi's event history.

## 7. Public status page (port 8082, root path)

- Enabled by default (setting). Shows only endpoints marked public: name, group, state, 60-minute strip, uptime summary.
- Never shows internal hostnames, IPs, error bodies or admin links.
- Categories (the endpoints' groups): Settings → Public pages sets their order, hides single categories,
  or turns categories off for one list sorted by name. Hidden categories are left out on the server, not by the page.
  The admin Status page uses the same order when grouped by group and never hides anything.
- Failed refresh shows a visible "data may be stale since …" banner rather than silently showing empty/stale data.
- `GET /api/public/status` (cached 15 s).

## 8. Public app dashboard (port 8081, root path)

Heimdall-style launcher of selected entries.

- Sources: services with dashboard enabled (display name defaults to service name); manual DNS entries with dashboard
  enabled (display name required).
- Cards link to the public URL; each shows its app icon (§9.1).
- Search field always shown (bug 146); sort selector (name, group, status; default from settings).
- Header counts: `x / n hosts online`, `x / n Linux firewall hosts available`.
- No admin controls. Enabled by default (setting).
- `GET /api/public/apps` (cached 15 s).

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
Telegram/Discord/SMTP against recorded contracts or local fakes.
