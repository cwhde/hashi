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

## 7. Public status page (port 8082, root path)

- Enabled by default (setting). Shows only endpoints marked public: name, group, state, 60-minute strip, uptime summary.
- Never shows internal hostnames, IPs, error bodies or admin links.
- Failed refresh shows a visible "data may be stale since …" banner rather than silently showing empty/stale data.
- `GET /api/public/status` (cached 15 s).

## 8. Public app dashboard (port 8081, root path)

Heimdall-style launcher of selected entries.

- Sources: services with dashboard enabled (display name defaults to service name); manual DNS entries with dashboard
  enabled (display name required).
- Cards link to the public URL; icon from the asset set or per-entry upload.
- Search collapsed by default; sort selector (name, group, status; default from settings).
- Header counts: `x / n hosts online`, `x / n Linux firewall hosts available`.
- No admin controls. Enabled by default (setting).
- `GET /api/public/apps` (cached 15 s).

## 9. Assets

Default assets from `https://static.juzo.io/`, downloaded once and cached under `/data/assets`: icons `/icon/default/`,
`/icon/bloom/`; logos `/logo/2k/`, `/logo/4k/`; backgrounds `/background/2k/`, `/background/4k/`; media
`/media/chillwave-mix.mp3`. Logos/icons on by default; backgrounds optional and off by default (admin screens use the theme canvas; a
user-selected background is a personalization option that must not reduce readability); all replaceable in Settings → Appearance.

## 10. Tests that must exist

Check executors against local test servers (HTTP/H2C/HTTPS/TCP/UDP/DNS); state machine thresholds; rollup correctness
with closed-bucket rule; public APIs expose only selected fields (schema test); notification routing + cooldown;
Telegram/Discord/SMTP against recorded contracts or local fakes.
