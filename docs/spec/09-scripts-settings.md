# 09 — Custom host scripts and settings

## 1. Scripts

Privileged shell scripts managed for Linux firewall hosts.

Fields: name, description, body (CodeMirror shell editor), enabled, optional cron expression, target hosts (default: all
firewall hosts; an explicit set where every target is disabled means **no** hosts, never "fall back to all"), run timeout (honoured end-to-end for manual and scheduled runs), environment variables
(secret ones in the vault), manual trigger.

Sync:

- Scripts written to `/opt/hashi/scripts/<slug>.sh`, root-owned, mode 0750, never world-writable; manifest
  `/opt/hashi/scripts/manifest.json` with SHA-256 per script.
- Schedules: systemd timers (preferred; `OnCalendar` converted from cron with a round-trip test that proves equivalent
  firing times) or `/etc/cron.d/hashi-scripts`.
- Scheduled runs use a Hashi wrapper (`/opt/hashi/scripts/run.sh <slug>`) that loads the env file (0600), enforces the
  timeout, and writes output to a spool that Hashi collects — so scheduled runs get the same env, redaction and history as
  manual runs.
- Manual runs execute over SSH; stdout/stderr captured (capped, e.g. 1 MB), redacted (known secret values replaced), stored.

Safety: saving or running requires reauth + `scripts.manage`; UI shows target hosts and diff before applying; scripts
cannot run when needed credentials are locked; every run is audited.

API: `/api/scripts` CRUD, `POST /api/scripts/{id}/plan|apply|run`, `GET /api/scripts/{id}/runs`, `GET /api/script-runs/{runId}`.

## 2. Settings catalogue

Every setting has a runtime consumer and a test proving it changes behaviour; `internal/settings/consumers_test.go` lists the test for each one and fails for a setting without. The settings UI never saves values it failed to load.

Categories follow 10c §1.2, in the navigation's order. Each category is one component on the Settings page; a feature
page whose settings live here links to its category with a gear in its header.

| Category | Settings |
| --- | --- |
| General | root domain, admin domain, internal URL, default sync interval, timezone display preference |
| Security | admin sign-in: passkeys, vault mode (reauth), admin idle timeout (≤4 h), active sessions, bootstrap networks (private ranges only, 02 §1.1), trusted proxies (private ranges only, reauth, 01 §8) |
| Edge protection | the engine settings of 06-edge-security.md §10 (WAF default mode and violation threshold, L7 rate threshold, challenge expiry and max failures, L4 meters, staging timeout, host-link poll interval, ban function and counter decay, feed refresh and stale TTL), CAPTCHA (06 §11, clearance lifetime), edge sign-in session length/idle/remember-device. Sign-in providers and the GeoIP download source are connections (10c §1.2) |
| Services and Traefik | middlewares on every service (names from the user middleware file, 04 §4, added to every user HTTP service after Hashi's defaults; system services never). The file itself is edited on the Traefik page. Log paths are fixed because Hashi installs Traefik; ACME defaults belong to the certificate provider on Connections |
| DNS and agent names | internal agent DNS: enabled, domain (reauth), IP mode, stale behaviour. DNS itself has none: the default TTL is set per DNS connection (03), removing records is always its own high-risk plan, and an import always shows what it found. The DNS page shows agent names read-only |
| Firewall | NetBird mode a new host starts with, MSS clamping on NetBird-routed traffic and narrow bridges (05 §3) (allow/block and host access are IP set bindings on the Firewall page, not settings). Opening a port is always a high-risk plan, the interface is detected or entered and never assumed (05 §5), routed networks are per host |
| Pulse | heartbeat interval, stale threshold, update agents automatically (default on, ADR 0003) |
| Monitoring and notifications | monitor defaults (interval, timeout, allowed HTTP codes, latency thresholds, retention), notification channels, default route, cooldowns. Deliveries are history: Activity, or the check that sent them |
| Public pages | public dashboard on/off, default sort, visibility defaults; public status page on/off. Which service shows on a page is an instant switch on App display |
| Appearance | theme, custom theme, reduced transparency (all save on choice, 10c §5.1). Logo, icon and public page assets wait for the asset system (07 §9), overview widgets for the overview page (bugs 30, 31) |

API: `GET /api/settings`, `GET|PUT /api/settings/{category}`.
