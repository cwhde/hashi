# 03 — DNS: provider, managed records, AdGuard, internal agent DNS

## 1. Provider interface

```go
type Provider interface {
    Capabilities(ctx) (Capabilities, error)          // record types, TTL limits, multi-value support, comments
    ListZones(ctx) ([]Zone, error)
    ResolveZone(ctx, fqdn string) (Zone, error)       // longest-suffix match
    ListRRSets(ctx, zone Zone) ([]RRSet, error)
    Plan(ctx, zone Zone, desired, observed []RRSet) (Plan, error)
    Apply(ctx, zone Zone, plan Plan) (Result, error)  // idempotent; "no change" = success
    Verify(ctx, zone Zone, plan Plan) (Result, error)
}
```

`Capabilities` includes an optional **ownership marker** (`MarkOwned` / `IsMarkedOwned`) that each provider implements
in its own way (Hetzner: RRSet labels; another provider: record comments or tags; none: unsupported → skipped). Core DNS
logic only ever asks the interface; provider-specific fields never leak past the adapter, so adding or swapping a
provider stays a new module and nothing else.

Hashi manages a single zone (the root domain's zone, chosen in setup). The interface stays zone-aware so a
provider module doesn't need to change if multi-zone support is ever added, but multi-zone is not a requirement.
"Full custom domain" services must lie inside that zone.

The unit of ownership and change is the **RRSet** (name + type → TTL + values). Hashi's UI shows records, but plans are
RRSet operations. Future providers (Cloudflare, Route53, PowerDNS, …) implement the same interface without changes to
service logic.

## 2. Hetzner (first provider)

Hetzner shut down the legacy DNS Console/API (`dns.hetzner.com`, `Auth-API-Token`) on 2026-05-27. DNS now lives in the
**Hetzner Cloud API** (`api.hetzner.cloud/v1`, `Authorization: Bearer <project token>`) as Zones and RRSets, with some
operations returning asynchronous Actions.

**Hard rule:** the adapter is written from recorded real responses, not from memory. Before any adapter code:

1. Run `make contract-capture PROVIDER=hetzner` (read-only calls: list zones, get zone, list RRSets with pagination) against the real project token.
2. Sanitized responses land in `testdata/contracts/hetzner/` with the date and API docs URL.
3. Write operations are captured once against a disposable test record (`_hashi-test.<zone>` TXT) in a user-approved plan.
4. Adapter tests replay these fixtures; a scheduled CI job (manual dispatch + weekly) re-runs the read-only capture and
   diffs response *shapes* to detect API drift.

Adapter requirements:

- Token validation = list zones; a legacy DNS token yields an actionable error ("create a Cloud project API token with
  read/write for DNS").
- Pagination handled until exhausted; respect `RateLimit-*` headers with backoff; retries only for idempotent operations.
- Async Actions are polled until success/failure with timeout; failure surfaces the action error.
- TXT values are normalized (quoting/chunking per the API's documented format, verified by the captured fixtures).
- TTL preserved unless Hashi owns the RRSet or the user changes it.

**Changed from V2:** V2's adapter targeted the shut-down API and was never run against a real token.

## 3. Record types and ownership

Manual DNS tab minimum: A, AAAA, CNAME, MX, TXT. More types (CAA, SRV, …) only if capability discovery reports them.

Ownership (`dns_ownership`): per RRSet, origin ∈ `system` | `managed` | `imported` | `user`, plus `last_applied_hash`.
The database is authoritative. If the provider supports an ownership marker (§1), Hashi also writes it so ownership is
visible in the provider console and can be re-imported after a DB loss; a marker alone never grants ownership. RRSets the
provider reports as change-protected are read-only to Hashi. A "zone default" TTL is preserved as such.
(Hetzner adapter: marker = RRSet label `hashi.io/owner=<instance-id>`, protection = `protection.change`, default TTL = `ttl: null`.)

Conflicts: if Hashi wants an RRSet whose name+type exists but is not owned, the plan contains a `conflict` item — never
an overwrite. Resolving requires an explicit adopt (import) decision. CNAME exclusivity is validated before planning
(no CNAME alongside other types at the same name; no CNAME at the apex).

Never: delete or modify `NS`/`SOA`; delete unknown records outside the prune workflow.

## 4. Import

During setup and later (DNS → Import):

- Truncating table of existing RRSets: name, type, TTL, values (truncated, expandable). Tokens never shown.
- Per row: **import as managed manual entry** / **leave unmanaged** (default) / **mark for prune**.
- Prune is a separate high-risk plan with explicit confirmation listing every RRSet; `NS`/`SOA` are not selectable.
- Imported entries become `imported` origin; the user may later convert them to services or link them to monitoring/dashboard.
- Decisions persist in `dns_import_decisions` so a re-import shows only new RRSets.

**IPv4 only for generated records.** Hashi never creates AAAA records on its own; AAAA exists only when the admin adds
one manually in the DNS tab. Reason: the hosts have no public IPv6, and a name with both A and AAAA lets browsers switch
between IPv4 and IPv6, which changes the client IP and breaks strictly IP-bound admin sessions (02 §3.2).

## 5. Generated records

### 5.1 Host records

For each Linux firewall host named `machine1` under root `example.com`:

| Name | Type | Target |
| --- | --- | --- |
| `machine1.example.com` | A | host public IPv4 |
| `via.machine1.example.com` | CNAME | `machine1.example.com` |
| `on.machine1.example.com` | CNAME | `via.machine1.example.com` (default) |

Routing override: `on.machine1` may point to `via.machine2` so traffic for services on machine1 enters through machine2.
AdGuard rewrites never use this override.

### 5.2 Service records

| Situation | Public record |
| --- | --- |
| Service maps to a detected firewall host (target IP in its managed subnets / NetBird-routed CIDRs / host FQDN) | `CNAME service.example.com → on.<host>.example.com` |
| No host detected | Resolve the service's target address set (05a-ip-sets.md §5.2) — single IP, Pulse agent (internal **and** public addresses), host, DNS name — and match every address against hosts; match → CNAME to `on.<host>`; no match → A with the resolved public IPv4 addresses |

A target that resolves to nothing keeps the last applied record (invariant 21).

Domain modes: root (apex — only A, no CNAME; Hashi flattens by resolving `on.<host>` to the host IP), subdomain, full custom domain (must be in a managed zone).

### 5.3 Certificates (Hashi-issued, distributed as files)

Hashi, not Traefik, obtains all certificates. It runs the ACME client itself (DNS-01 through its own DNS provider
adapter) and copies the resulting files to every Traefik host. Traefik hosts never talk to an ACME server and never
see the DNS token.

**Which certificates:** Hashi manages one zone, so it derives the certificate set from the names it serves:

- One certificate for the root domain and `*.<root>` (covers every first-level name, e.g. `app.example.com`).
- For every deeper parent that has served names, one more wildcard: a service `fra1.ns.example.com` makes Hashi add
  `*.ns.example.com`. Wildcards cover exactly one level, so the rule is "one wildcard per parent name in use".
- These are SANs of as few certificates as the CA allows (GTS/LE limit 100 names per certificate); a wildcard that is no
  longer needed is dropped at the next renewal, never mid-term.

**Lifecycle (job runner, job `certificates.renew`, daily):** issue when a needed name is missing; renew 30 days before
expiry; on failure retry with backoff and raise a warning (Overview + notification) at 21 days left, critical at 7 days.
`_acme-challenge` TXT records are written and removed through the normal provider adapter with origin `system` and are
excluded from prune. ACME account key and certificate private keys are vault secrets (service class).

**Distribution:** a sync plan per Traefik host writes `/etc/hashi/traefik/certs/<name>.crt|.key` (key mode 0600) and the
dynamic file `/etc/hashi/traefik/dynamic/hashi-certs.yaml` listing them under `tls.certificates`. Every host gets every
certificate; Traefik picks the matching one per SNI. Verify = TLS handshake to the host for one name per certificate and
compare the served fingerprint. Distribution is low-risk (no restart; the file provider reloads). Verify before
implementation that the pinned Traefik version reloads changed certificate files without a restart
(`make contract-capture PROVIDER=traefik`).

**UI:** Traefik → Certificates tab: each certificate with names, issuer, expiry, last renewal result, per-host
distribution state, "Renew now" (creates a plan). Runs appear in Activity → Jobs.

**Changed from the earlier draft:** Traefik hosts used to request their own certificates via lego's `httpreq` provider
pointed at Hashi. With several Traefik hosts each requesting the same names, one host's challenge cleanup deleted the
other's TXT value (random renewal failures), and every host held an ACME account. Central issuance removes the race,
the `httpreq` endpoint and per-host ACME storage, uses one ACME account (fewer rate-limit hits), and wildcards keep
subdomain names out of public certificate-transparency logs. Cost: certificates expire if Hashi is down for weeks
(warned from 21 days), and every edge host holds the wildcard keys — they already serve every service.

## 6. Sync rules

- Compare desired vs observed before every write; empty diff = no API writes.
- "No change" responses are success.
- Generated records are hidden from the manual DNS tab by default (toggle "show generated").
- Unknown records are never deleted outside the prune workflow.
- Plans show exact name/type/TTL/values before and after.

## 7. API

`GET /api/dns/zones`, `POST /api/dns/zones/{id}/refresh`, `GET /api/dns/zones/{id}/rrsets?origin=&q=`,
`POST /api/dns/records` · `PATCH|DELETE /api/dns/records/{id}` (manual entries),
`GET /api/dns/import/{zoneId}` · `PUT /api/dns/import/{zoneId}/decisions`,
`POST /api/dns/plan` · `POST /api/sync/plans/{id}/approve|apply`,
`GET /api/certificates` · `POST /api/certificates/{id}/renew` (creates a plan).

Manual entry fields: name, type, TTL, values, display name (required when dashboard or monitoring is enabled),
dashboard visible, monitoring enabled. A and AAAA values are either literal addresses or an **address set**
(05a-ip-sets.md §5.2: a Pulse agent, a host, a DNS name, single IPs — e.g. `home.example.com` A → Pulse agent `home`,
scope `public`); the record follows the set's resolution for its family and is re-planned when it changes. An empty
resolution keeps the last applied values (invariant 21).

## 8. AdGuard Home (optional)

Hashi manages DNS rewrites through AdGuard Home's control API (list/add/update/delete rewrites). The exact request and
response formats come from recorded fixtures.

Rules:

- Ownership is Hashi-side (`adguard_rewrites_owned`: domain + answer + origin + hash). AdGuard has no comments; Hashi
  never touches a rewrite it does not own.
- Desired rewrites: for each service with a detected firewall host, `service.example.com → <true internal Traefik IP of
  that host>`. The routing override is never applied internally.
- Duplicate cleanup: only duplicates of **owned** entries, and only ones shown in the preview.
- Disabled AdGuard connections are neither planned nor applied.
- AdGuard connection targets are address sets (05a §5.2), so an instance can be reached through its Pulse agent.
- Credentials: username + password (basic auth) or none; TLS verify on by default; optional path prefix.
- Multiple AdGuard instances are independent connections; Hashi writes its owned rewrites to every enabled instance
  itself, so no external sync tool is needed. (Migration only: while an existing sync tool still runs, connections it
  replicates to can be temporarily set to `read-only` to avoid two writers; see 11-delivery.md §7.)

## 9. Internal agent DNS (optional, requires AdGuard)

Stable internal names for Pulse agents via AdGuard rewrites under an internal domain, default `hashi.home.arpa`.
Example: `kanae.hashi.home.arpa → 10.0.0.42`.

- Only for Pulse agents; resolves to the agent's selected IP (setting: selected / private / public; default selected).
- Name normalization: lowercase ASCII slug, invalid chars → `-`, collapse repeats, trim, reject empty, prevent collisions
  (collision requires a manual per-agent override).
- Per-agent: DNS enabled, name override.
- Never creates Traefik routers for this domain and never implies HTTP services exist there; the UI says "DNS only, not reverse proxy".
- Coexists with real-domain internal rewrites; never replaces them; never touches manual rewrites.
- Stale agent default: keep last rewrite, mark stale in UI. Optional (off by default): remove after configured stale duration.
- Sync through the normal plan/apply with audit; deletions of owned agent rewrites are high-risk unless caused by explicit
  agent deletion.
- Settings: enable, domain (reauth to change), default IP mode, stale behaviour.
- API: `GET|PUT /api/settings/internal-agent-dns`, `POST /api/settings/internal-agent-dns/preview-sync|apply-sync`.

## 10. Tests that must exist

- Invariants 1–4 against fixtures (NS/SOA untouchable; unowned RRSets untouched; failed validation → no writes).
- RRSet mapping: multi-value, TTL change only, TXT quoting, CNAME exclusivity, apex flattening.
- Host record generation and routing override; Pulse IP matching → CNAME not A.
- AdGuard: manual rewrite preserved; duplicate of owned entry removed only when previewed; disabled connection untouched.
- Agent name normalization and collisions.
- Contract replay tests for every recorded endpoint.
