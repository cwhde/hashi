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

The unit of ownership and change is the **RRSet** (name + type → TTL + values). Hashi's UI shows records, but plans are
RRSet operations. Future providers (Cloudflare, Route53, PowerDNS, …) implement the same interface without changes to
resource logic.

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
The database is authoritative. Where the provider supports it, Hashi also writes a marker (Hetzner: RRSet label
`hashi.io/owner=<instance-id>`) so ownership survives a DB loss and is visible in the provider console; a label alone never
grants ownership. RRSets with provider-side change protection (Hetzner `protection.change: true`) are read-only to Hashi.
A `null` TTL means "zone default" and is preserved as such.

Conflicts: if Hashi wants an RRSet whose name+type exists but is not owned, the plan contains a `conflict` item — never
an overwrite. Resolving requires an explicit adopt (import) decision. CNAME exclusivity is validated before planning
(no CNAME alongside other types at the same name; no CNAME at the apex).

Never: delete or modify `NS`/`SOA`; delete unknown records outside the prune workflow.

## 4. Import

During setup and later (DNS → Import):

- Truncating table of existing RRSets: name, type, TTL, values (truncated, expandable). Tokens never shown.
- Per row: **import as managed manual entry** / **leave unmanaged** (default) / **mark for prune**.
- Prune is a separate high-risk plan with explicit confirmation listing every RRSet; `NS`/`SOA` are not selectable.
- Imported entries become `imported` origin; the user may later convert them to resources or link them to monitoring/dashboard.
- Decisions persist in `dns_import_decisions` so a re-import shows only new RRSets.

## 5. Generated records

### 5.1 Host records

For each Linux firewall host named `machine1` under root `example.com`:

| Name | Type | Target |
| --- | --- | --- |
| `machine1.example.com` | A (and AAAA if the host has public IPv6) | host public IP |
| `via.machine1.example.com` | CNAME | `machine1.example.com` |
| `on.machine1.example.com` | CNAME | `via.machine1.example.com` (default) |

Routing override: `on.machine1` may point to `via.machine2` so traffic for resources on machine1 enters through machine2.
AdGuard rewrites never use this override.

### 5.2 Resource records

| Situation | Public record |
| --- | --- |
| Resource maps to a detected firewall host (target IP in its managed subnets / NetBird-routed CIDRs / host FQDN) | `CNAME resource.example.com → on.<host>.example.com` |
| No host detected: manual IP | First try to match the IP to a host (as above). Only if no match: A/AAAA to that IP |
| No host detected: Pulse agent | Evaluate the agent's current internal **and** public IPs against hosts; match → CNAME to `on.<host>`; no match → A/AAAA with the agent's public IP |

Domain modes: root (apex — only A/AAAA, no CNAME; Hashi flattens by resolving `on.<host>` to the host IP), subdomain, full custom domain (must be in a managed zone).

### 5.3 ACME DNS-01

Traefik obtains certificates via DNS-01. Traefik hosts do **not** receive the DNS provider token. Instead, each
Traefik host's ACME resolver uses lego's `httpreq` provider pointed at Hashi (`/api/acme/httpreq/present|cleanup` on the
admin listener, internal address), authenticated with a per-Traefik-host credential stored in the vault. Hashi
validates that the requested `_acme-challenge` name belongs to a resource or system domain Hashi manages, writes the TXT
via the provider adapter, and removes it on cleanup. These records have origin `system` and are excluded from prune.

**Changed from V2:** V2 put the Hetzner token on every Traefik host for lego's legacy Hetzner provider. This keeps one
token in one place, works with the new Cloud API through Hashi's own adapter, and limits Traefik hosts to their own domains.
Verify before implementation: the pinned Traefik version's lego build includes `httpreq` (`make contract-capture PROVIDER=traefik-lego`).

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
`POST /api/acme/httpreq/present|cleanup` (Traefik credential auth).

Manual entry fields: name, type, TTL, values, display name (required when dashboard or monitoring is enabled),
dashboard visible, monitoring enabled, target source (manual / firewall host / Pulse agent).

## 8. AdGuard Home (optional)

Hashi manages DNS rewrites through AdGuard Home's control API (list/add/update/delete rewrites). The exact request and
response formats come from recorded fixtures.

Rules:

- Ownership is Hashi-side (`adguard_rewrites_owned`: domain + answer + origin + hash). AdGuard has no comments; Hashi
  never touches a rewrite it does not own.
- Desired rewrites: for each resource with a detected firewall host, `resource.example.com → <true internal Traefik IP of
  that host>`. The routing override is never applied internally.
- Duplicate cleanup: only duplicates of **owned** entries, and only ones shown in the preview.
- Disabled AdGuard connections are neither planned nor applied.
- AdGuard connections support agent-bound targets (08-pulse.md §5).
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
- Never creates Traefik routers for this domain and never implies HTTP resources exist there; the UI says "DNS only, not reverse proxy".
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
