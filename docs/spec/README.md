# Hashi — Implementation Specification (v3 restart)

Hashi (橋, "bridge") is a single-user control plane for a distributed homelab edge. One interface manages
public DNS, reverse-proxy (Traefik) configuration, Linux firewall forwarding, internal DNS rewrites,
service monitoring, public app/status pages, edge SSO, abuse response, host scripts and dynamic
endpoint discovery — without hand-editing DNS zones, Traefik files, firewall scripts or monitor configs.

This specification replaces the V2 specification and its three addenda (2025-12 → 2026-06). Every
feature from those documents is preserved; the mechanisms, stack and delivery process are new. Where a
V2 mechanism was replaced, the area file says so under **Changed from V2** with the reason.

## Spec map

Read only the file for the area you are working on, plus this index.

| File | Area |
| --- | --- |
| [01-platform.md](01-platform.md) | Stack, architecture, repo layout, data store, sync engine, jobs, API conventions |
| [02-setup-auth.md](02-setup-auth.md) | Setup wizard, bootstrap, passkeys, admin sessions, secrets vault |
| [03-dns.md](03-dns.md) | DNS provider (Hetzner Cloud), managed records, AdGuard Home, internal agent DNS |
| [04-traefik-resources.md](04-traefik-resources.md) | Resources, routes, Traefik install/static/dynamic config, ACME |
| [05-firewall.md](05-firewall.md) | Linux firewall hosts, generated script, NetBird compatibility, rescue rollback |
| [06-edge-security.md](06-edge-security.md) | Forward auth, edge SSO, rules, GeoIP, WAF, abuse engine, CAPTCHA, blocklists, security UI |
| [07-monitoring-public.md](07-monitoring-public.md) | Status monitoring, notifications, public status page, public app dashboard |
| [08-pulse.md](08-pulse.md) | Hashi Pulse agent, agent-bound connection targets |
| [09-scripts-settings.md](09-scripts-settings.md) | Custom host scripts, settings catalogue |
| [10-ui.md](10-ui.md) | Design intent, four themes, custom themes, navigation, quality bar |
| [11-delivery.md](11-delivery.md) | Delivery slices, prod-proof gates, testing, CI/CD, V1 cutover, operations |
| [06a-security-data-model.md](06a-security-data-model.md) | Security tables in full (verbatim from V2) |
| [10a-ui-references.md](10a-ui-references.md) | Descriptions of the UI reference screenshots |
| [10b-ui-system.md](10b-ui-system.md) | UI system from S0b: tokens, shell, page inventory, component rules (mocks at `/design`) |
| [environment.md](environment.md) | Real infrastructure facts (generic); host inventory lives outside the repo |

## Product principles

1. **Secure by default.** Passkey admin auth, encrypted secrets, audited privileged actions, conservative destructive operations.
2. **Lightweight.** One container, one binary, one embedded database. Mainstream, maintained libraries.
3. **Idempotent.** Every sync builds desired state, diffs it against observed state, applies only real changes.
4. **Extensible.** Providers sit behind interfaces; resource logic never depends on a specific provider.
5. **An operations tool, not a demo.** Dense, scannable, predictable, recoverable workflows.
6. **Honest about risk.** Anything that can replace configs, open ports, delete records or block traffic is previewed with its exact effect before apply.
7. **Proven against reality.** A feature is not done until it has worked against the real system it manages (see 11-delivery.md).

## Invariants (non-negotiable)

These hold in every slice. Each has at least one automated test named `TestInvariant_<n>_*`.

1. Hashi modifies only objects it created, imported, or the user explicitly assigned to it. Ownership is stored in Hashi's database; deterministic naming is a secondary signal, never the only one.
2. Hashi never deletes or modifies DNS `NS` or `SOA` records.
3. Hashi never writes provider state when read-only validation fails.
4. Hashi never runs live provider write tests (create/delete probe records) without an explicit user-confirmed plan.
5. Hashi never logs or returns secrets: tokens, private keys, passphrases, recovery keys, session IDs, decrypted provider config, Cap/OIDC/AdGuard/SSH/DNS credentials.
6. All timestamps are stored in UTC; the browser renders them in its locale.
7. Traefik, firewall, DNS and AdGuard state change only through a sync plan with preview → apply → result → audit entry.
8. Files are written atomically: render → validate → hash-compare → write to unique temp path → rename, only when content changed. Identical Traefik files are never rewritten (no hot-reload churn).
9. Every generated object carries an origin: `system`, `managed`, `imported` or `user`.
10. System resources are edited only through their owning workflow and cannot be deleted while required.
11. After setup, minimums are enforced: ≥1 DNS provider, ≥1 Traefik connection, ≥1 Linux firewall host. AdGuard, notification providers, SSO providers, CAPTCHA and Pulse agents are optional.
12. Firewall rules live only in Hashi-owned chains and sets. Hashi never flushes global or foreign state (including NetBird's).
13. Every external write adapter implements `Plan`, `Apply`, `Verify`.
14. Setup is resumable after restart and never reports "configured" before passkey + recovery key are confirmed.
15. Every background job exposes last run, next run, status, duration, diff summary and error.
16. Routine reconciliation runs with no browser session. Risky changes found by background sync become pending plans awaiting approval; safe changes continue.
17. Custom scripts are root-capable operations: passkey reauthentication to save or run, full audit.
18. Critical security, crypto, SSH, DNS, OIDC, WebAuthn and YAML handling use proven libraries or the standard library — never hand-rolled.
19. The repository is public: no secret, real token, private host inventory or production config is ever committed.

## Glossary

| Term | Meaning |
| --- | --- |
| Resource | An externally exposed service (HTTP/HTTPS/H2C/TCP/UDP) Hashi routes to. |
| Connection | A configured external system: DNS provider, Traefik host, firewall host, AdGuard, OIDC provider, notification provider, Cap, NetBird API. |
| Firewall host | A Linux machine with a public IP that forwards traffic into managed subnets (typically a Proxmox host). |
| Traefik host | A machine running a Hashi-managed Traefik instance. Every Traefik host receives every enabled resource. |
| Sync plan | A persisted diff between desired and observed state for one subsystem/connection, with a risk level. |
| Origin | `system` / `managed` / `imported` / `user` marker on generated objects. |
| Pulse agent | Small Go agent reporting a host's current public/private IPs. |
| Security subject | IP, CIDR, ASN, country, region, session or composite key that Hashi can observe/challenge/block. |
| Prod-proof | The recorded evidence that a slice worked against the real system (11-delivery.md). |

## Open questions

Tracked here until answered; answers move into the relevant area file.

None right now.

Answered by the 2026-10-27 survey: Hashi replaces Pangolin, both current Traefik instances, Gatus, adguardhome-sync and the
host firewall script with its own installs (11-delivery.md §7); the home host is an `external_nat` edge host (05-firewall.md §1.1).

## Reference links

- [Svelte docs](https://svelte.dev/docs/svelte/overview)
- [shadcn-svelte docs](https://www.shadcn-svelte.com/docs)
- [Bits UI docs](https://www.bits-ui.com/docs)
- [Lucide icons](https://lucide.dev/)
- [Traefik file provider](https://doc.traefik.io/traefik/master/reference/install-configuration/providers/others/file/)
- [Traefik forward auth](https://doc.traefik.io/traefik/v3.4/middlewares/http/forwardauth/)
- [Traefik rate limit](https://doc.traefik.io/traefik/v3.4/middlewares/http/ratelimit/)
- [Traefik Coraza WAF plugin](https://plugins.traefik.io/plugins/65f2aea146079255c9ffd1ec/coraza-waf)
- [Hetzner Cloud API — DNS zones and RRSets](https://docs.hetzner.cloud/reference/cloud) (the legacy DNS API is shut down)
- [AdGuard Home OpenAPI](https://github.com/AdguardTeam/AdGuardHome/blob/master/openapi/openapi.yaml)
- [Gatus endpoint model](https://gatus.io/docs/endpoints)
- [MDN WebAuthn](https://developer.mozilla.org/en-US/docs/Web/API/Web_Authentication_API)
- [MDN WebAuthn extensions and PRF](https://developer.mozilla.org/docs/Web/API/Web_Authentication_API/WebAuthn_extensions)
- [MaxMind GeoLite2 database docs](https://dev.maxmind.com/geoip/docs/databases/)
- [MaxMind database update docs](https://dev.maxmind.com/geoip/updating-databases)
- [NetBird routing peers](https://docs.netbird.io/manage/networks/how-routing-peers-work)
- [NetBird ports and firewalls](https://docs.netbird.io/about-netbird/ports-and-firewalls)
- [NetBird CLI](https://docs.netbird.io/get-started/cli)
- [Hashi static asset host](https://static.juzo.io/)
- [Go WebAuthn](https://github.com/go-webauthn/webauthn)
- [sqlc](https://docs.sqlc.dev/)
- [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen)
- [lego httpreq provider](https://go-acme.github.io/lego/dns/httpreq/)
