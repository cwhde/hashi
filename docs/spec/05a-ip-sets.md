# 05a — IP sets: the one address primitive

Every place in Hashi that needs an address — "who is this request from" or "where does this go" — takes an **IP set**.
Firewall allow/block, host access, rule `Applies To`, DNS record values, service upstreams and connection targets all
point at IP sets; feeds and the abuse engine's dynamic state are IP sets too. So anything can be "set via Pulse", a
whitelist can contain a Pulse-tracked home IP, and one change (a new WAN IP) reaches every consumer through one path.
Decision record: [ADR 0002](../adr/0002-security-model.md).

## 1. Terms

| Term | Meaning |
| --- | --- |
| IP item | A leaf: one address, one prefix, or a reference that resolves to addresses/prefixes (§2) |
| IP set | A named, ordered list of elements; an element is an IP item or a reference to another IP set |
| Resolve / compile | Turn a set into its current prefixes (IPv4 and IPv6 separately) plus geo predicates (§4) |
| Match position | A consumer that asks "is this address in the set?" (§5.1) |
| Target position | A consumer that needs concrete addresses to send traffic or answers to (§5.2) |
| Address set | A set that is valid in a target position: every leaf resolves to single addresses (§5.2) |

## 2. IP items

Every item has: `type`, type-specific fields, optional `note`, optional `expires_at` (an expired item drops out of the
set at the next compile; an Activity event records it), `created_at`, `created_by`.

| Type | Fields | Resolves to | Match | Target |
| --- | --- | --- | --- | --- |
| `ip` | address (IPv4 or IPv6) | that address (/32 or /128) | yes | yes |
| `cidr` | prefix (normalized: host bits zero; `/32`, `/128` stored as `ip`) | that prefix | yes | no |
| `pulse` | agent, scope `public` \| `internal` \| `all`, optional internal selector (interface name or CIDR) | `public`: the heartbeat's source address; `internal`: the agent's private candidates, selected one first (selector narrows); `all`: both | yes | yes |
| `host` | firewall/edge host, scope `public` \| `internal` \| `all` | `public`: the host's detected (or overridden) public IPv4/IPv6; `internal`: its NetBird and bridge addresses detected at validation | yes | yes |
| `dns` | FQDN | its A/AAAA answers via Hashi's resolver, refreshed at the record TTL (min 60 s, max 1 h) | yes | yes |
| `geo` | kind `country` (ISO 3166-1) \| `region` (ISO 3166-2, needs a City database) \| `asn` | L7: GeoIP lookup of the request address. Firewall: the database's prefix list for that value (§4.3) | yes | no |

A set element of kind `set` references another IP set (by ID). References are acyclic: every save that adds a
reference runs a DFS over the set graph and rejects a cycle with `400` naming the path (`a → b → a`).

**Pulse agent states:** a `stale` agent keeps resolving to its last reported addresses (the set shows a warning); a
`revoked` or deleted agent resolves to nothing. Deleting an agent, host or set that an item references is blocked while
referenced (the error lists the referencing sets).

## 3. IP set kinds and what can be edited

| Kind | Examples | Elements | Metadata | Delete |
| --- | --- | --- | --- | --- |
| `user` | `trusted-devices`, `switzerland`, `office` | editable | editable | when unused |
| `system` | `all` (`0.0.0.0/0`, `::/0`); `hashi-hosts` (a `host` item, scope `public`, for every registered firewall/edge host, maintained automatically); `private-networks` (RFC 1918, RFC 6598, loopback, link-local, ULA) | read-only | name/description read-only | never |
| `feed` | `feed-spamhaus-drop`, custom URL feeds (06-edge-security.md §8) | read-only (owned by the fetcher) | feed settings editable | when unused |
| `dynamic` | `dynamic-challenge`, `dynamic-blocked` (06-edge-security.md §3) | entries owned by the engine; manual actions: remove entry, add entry with a duration ("challenge now", "ban now"); counters are per address (06 §5.1) | read-only | never |

All kinds are the same object (one table, one type); `kind`, `origin` and `locked` flags decide what the user may edit
(06a §0). User and system sets may contain any other kind (a user set can include a feed set or `hashi-hosts`). Dynamic sets
contain only engine entries: an IPv4 address or an IPv6 /64, with source, reason, created, expires (none = permanent).

## 4. Resolution (the compiler)

### 4.1 Output

`Compile(set) → { v4 prefixes, v6 prefixes, geo predicates, status, problems }`. Prefixes are normalized, merged and
sorted (interval list). `status` is `ok`, `partial` (some items could not resolve: stale agent, DNS failure, GeoIP missing,
feed stale) or `empty`. `problems` names each failing item. Membership tests at L7 use the compiled interval lists
(binary search) plus a GeoIP lookup for geo predicates — p99 well under the forward-auth budget (06 §2.2).

### 4.2 Reactivity

Hashi keeps the reverse dependency graph (item source → sets → consumers). A change in any source recompiles the
affected sets and notifies their consumers, debounced 2 s like other triggers (01-platform.md §5.4):

| Source change | Example |
| --- | --- |
| Pulse heartbeat with new addresses | home WAN IP changes at 3 AM |
| Host public IP detected/overridden | VPS re-provisioned |
| DNS item refresh with new answers | dynamic DNS name updated |
| Feed fetch with new content | Spamhaus DROP updated |
| GeoIP database update | monthly MaxMind release |
| Item added/removed/expired, set edited | user adds a device |
| Dynamic entry added/removed/expired | engine ban, manual unban |

Consumers react by queuing their own sync: firewall (folded sets, 05-firewall.md §4), DNS records, Traefik upstreams,
connection targets. Dynamic entries reach the firewall through the host link (05 §6) instead of a plan.

### 4.3 Geo at the firewall

`geo` items in a firewall-bound set are expanded from the MaxMind database (iterate the MMDB networks, keep those whose
country/region/ASN matches). Countries can be tens of thousands of prefixes; the plan preview shows counts per set and
host, not every prefix. Without a GeoIP database `geo` items resolve to nothing, the set is `partial`, and binding a set
with geo items to a firewall position needs a confirmation that names the missing database.

## 5. Positions

### 5.1 Match positions (any set)

| Consumer | Spec |
| --- | --- |
| Global firewall allow / block (lists of sets) | 05-firewall.md §4 |
| Host access per firewall host | 05-firewall.md §3 |
| Rule item `Applies To` | 06-edge-security.md §4 |

An empty resolution matches nothing. When a set bound to firewall allow or host access becomes empty or loses the
address of the current admin session, the change is high-risk (lock-out guard, 01-platform.md §5.2).

### 5.2 Target positions (address sets only)

A set is an **address set** when every leaf is `ip`, `pulse`, `host` or `dns` — no `cidr`, `geo`, `all`, feed or dynamic
set, directly or through nesting. The compiler reports this as a capability; target positions refuse anything else, and
editing a set that is used as a target refuses changes that would make it stop being an address set (the error lists the
consumers).

| Consumer | Uses | Spec |
| --- | --- | --- |
| DNS record values (A / AAAA) | all resolved addresses of the record's family; services keep the CNAME-to-host rule | 03-dns.md §5.2, §7 |
| Service upstream targets | all resolved addresses, load-balanced | 04-traefik-services.md §1.1 |
| Connection targets (Traefik host SSH, firewall host SSH, AdGuard, future connections) | resolved addresses in item order: first is primary, the rest are tried in order | 08-pulse.md §5 |
| Internal agent DNS | the agent's addresses per the setting | 03-dns.md §9 |

**Never empty applied state:** if a target resolves to no address (agent never seen, DNS failure), the consumer keeps
its last applied value, is marked `target unresolved` with the reason, and shows on Overview → needs attention. Nothing
is deleted or emptied because a resolution failed (invariant 21).

## 6. Search and the address page

- **Inside a set:** search by address (shows items that contain it, including through nested sets), by prefix
  (overlapping items) and by note text. Paginated; feed and dynamic sets can be large.
- **Address lookup** (`/ip-sets/address?ip=`, also from ⌘K): every set that contains the address with the path through
  nesting (`trusted-devices → phone (pulse, public)`), the folded firewall verdict per firewall host with the deciding
  prefix and set (05 §4), dynamic state (challenge with failure count, ban with expiry), ban and challenge counters with
  edit/reset (06 §5.1), a "try a request"
  trace through a chosen service's rules (06 §4.3), recent events, and actions (add to a set, challenge now, ban now,
  remove from dynamic sets, edit/reset counters).

## 7. Implementation shape

IP sets and items are first-class typed objects in every layer, so the rules above are enforced by types rather than by
convention:

- **Database:** `ip_sets` and `ip_set_elements` with a `type` column and per-type `CHECK` constraints (06a §1). No
  consumer stores raw addresses for something an IP set can express.
- **Go:** `internal/ipsets` owns the item variants (one Go type per variant behind a sealed interface), the cycle check
  and the only compiler. Consumers receive compiled results; they never parse items themselves.
- **API:** items are an OpenAPI `oneOf` with discriminator `type`; consumers reference sets by ID (`ipSetId`), never
  inline addresses. The generated TypeScript client carries the same union.
- **UI:** shared components — IP set picker (filters to address sets in target positions, shows kind and size), item
  editor (type switch, per-type fields), set badge, address link (opens the address page). Every page that takes an
  address uses these components.

## 8. API

`/api/ip-sets` CRUD (`?kind=`, `?q=`) · `GET /api/ip-sets/{id}/elements?q=&cursor=` · `POST /api/ip-sets/{id}/elements`
· `PATCH|DELETE /api/ip-sets/{id}/elements/{elementId}` · `GET /api/ip-sets/{id}/resolved?family=&cursor=` ·
`GET /api/ip-sets/{id}/consumers` · `POST /api/ip-sets/{id}/entries` (dynamic: challenge/ban now, duration) ·
`DELETE /api/ip-sets/{id}/entries/{address}` · `GET /api/addresses/{address}` (lookup) ·
`PATCH /api/addresses/{address}/counters` (06 §5.1, §14) · `POST /api/addresses/{address}/trace` (service + path → Stage A/B trace).

Scopes: read with any admin session; edits need `security.manage`; edits to sets bound to firewall positions also create
a firewall plan (05 §4.4).

## 9. Audit

Set create/update/delete; element add/edit/remove; manual dynamic actions (challenge now, ban now, remove); counter
edits and resets; binding changes in any position.

## 10. Tests that must exist

- Normalization of every item type (mapped IPv4-in-IPv6, host bits, `/32` → `ip`), invalid input rejected.
- Cycle detection (direct, indirect, self) with the reported path; deep nesting compiles.
- Compiler: union and merge, v4/v6 split, `partial`/`empty` status per failing source, expiry drop-out.
- Address-set capability through nesting; target consumers reject non-address sets; editing a used target set refuses
  a `cidr`/`geo` addition.
- Reactivity: a Pulse heartbeat with a new public address recompiles every dependent set and queues exactly the dependent
  syncs (firewall, DNS, Traefik, connection).
- `TestInvariant_21_UnresolvedTargetKeepsAppliedState`: DNS record, upstream and connection target keep their last value
  when resolution becomes empty.
- `TestInvariant_22_SetGraphsAcyclic` (with rule sets, 06 §4).
- Geo expansion from a fixture MMDB; missing database → `partial`.
