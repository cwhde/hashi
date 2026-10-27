# 09 — Custom host scripts and settings

## 1. Scripts

Privileged shell scripts managed for Linux firewall hosts.

Fields: name, description, body (CodeMirror shell editor), enabled, optional cron expression, target hosts (default: all
firewall hosts; an explicit set where every target is disabled means **no** hosts, never "fall back to all" — V2 bug
AUD-166), run timeout (honoured end-to-end for manual and scheduled runs — V2 bug AUD-110), environment variables
(secret ones in the vault), manual trigger.

Sync:

- Scripts written to `/opt/hashi/scripts/<slug>.sh`, root-owned, mode 0750, never world-writable; manifest
  `/opt/hashi/scripts/manifest.json` with SHA-256 per script.
- Schedules: systemd timers (preferred; `OnCalendar` converted from cron with a round-trip test that proves equivalent
  firing times — V2 bug AUD-080 changed timing) or `/etc/cron.d/hashi-scripts`.
- Scheduled runs use a Hashi wrapper (`/opt/hashi/scripts/run.sh <slug>`) that loads the env file (0600), enforces the
  timeout, and writes output to a spool that Hashi collects — so scheduled runs get the same env, redaction and history as
  manual runs (V2 bugs AUD-010, AUD-111).
- Manual runs execute over SSH; stdout/stderr captured (capped, e.g. 1 MB), redacted (known secret values replaced), stored.

Safety: saving or running requires reauth + `scripts.manage`; UI shows target hosts and diff before applying; scripts
cannot run when needed credentials are locked; every run is audited.

API: `/api/scripts` CRUD, `POST /api/scripts/{id}/plan|apply|run`, `GET /api/scripts/{id}/runs`, `GET /api/scripts/runs/{runId}`.

## 2. Settings catalogue

Every setting has a runtime consumer and a test proving it changes behaviour (V2 bug AUD-141: settings persisted that no
code read). The settings UI never saves values it failed to load (V2 bug AUD-142).

| Category | Settings |
| --- | --- |
| General | root domain, admin domain, internal URL, default sync interval, timezone display preference, trusted proxies (reauth) |
| Security | admin idle timeout (≤4 h), edge SSO session length/idle/remember-device, adaptive defaults, WAF defaults, ban policies, block TTLs, challenge thresholds, GeoIP update, vault mode (reauth) |
| Appearance | theme, custom theme, logo, icon, public page assets, overview widget order/visibility, reduced transparency |
| Monitoring | default interval, timeout, allowed HTTP codes, latency thresholds, retention, public status enabled |
| Dashboard | public dashboard enabled, default sort, visibility defaults |
| DNS | default TTL, prune policy, import behaviour |
| Traefik | config paths, log paths, ACME defaults, pinned version display, middleware editor options |
| Firewall | trusted CIDRs, port confirmation behaviour, persistence mode, NetBird default mode, default interface, routed-network behaviour, MSS clamping |
| Notifications | default provider/route, cooldowns |
| Pulse | heartbeat interval, stale threshold |
| Internal agent DNS | enabled, domain (reauth), IP mode, stale behaviour |
| CAPTCHA | see 06-edge-security.md §7 |

API: `GET /api/settings`, `GET|PUT /api/settings/{category}`.
