# 06a — Security data model: object model, tables, Go packages

Replaces the V2 tables carried over verbatim in the first spec (subjects, subject states, manual entries, blocklist
tables, firewall block tracking — none of them was built). Behaviour is in [05a-ip-sets.md](05a-ip-sets.md),
[05-firewall.md](05-firewall.md) and [06-edge-security.md](06-edge-security.md); the migration files are the
authoritative schema. Timestamps are integer Unix milliseconds (`_at`), IDs ULIDs (01-platform.md §4.1).

## 0. Object model: shared primitives, layer-specific compilation

Where the concept is the same, the type, the storage and the logic are the same. Layers may compile a shared object into
their own structure (an interval list for forward auth, nftables sets for the kernel), but they never own a second model
of it. Differences between "kinds" of the same thing are data (`kind`, `origin`, lock flags), not separate types.

### 0.1 Shared — one type, one table, one implementation

| Concept | Stored as | Go | One implementation of |
| --- | --- | --- | --- |
| IP set (user, system `all`/`hashi-hosts`/`private-networks`, feed, dynamic) | `ip_sets` (`kind`, `origin`, `system_key`) | `ipsets.Set` | CRUD, cycle check, compiler, search |
| IP item (`ip`, `cidr`, `pulse`, `host`, `dns`, `geo`) and set reference | `ip_set_elements` (`type` + per-type columns + `CHECK`) | `ipsets.Item` sealed interface, one struct per variant | normalization, validation, resolution |
| Feed lines and engine entries | the same `ip_set_elements` rows (`ip`/`cidr`, `origin` = `feed` / `engine`) | `ipsets.Item` | same search, same compiler; engine-only data in a 1:1 side table (§2.3) |
| Rule set (reusable "group" sets, `global`, service root) | `rule_sets` (`kind`, `origin`) | `rules.Set` | CRUD, cycle check, flatten, evaluate |
| Rule item (incl. each service's default fallback) | `rule_entries` (`entry_type` item/set) | `rules.Item` | the same first-match evaluation; the fallback is an item with `origin = system` and locked fields |
| Compiled IP set | in memory (+ `compiled_hash` on the row) | `ipsets.Compiled` (v4/v6 interval lists, geo predicates, status, address-set capability) | one compiler; every consumer below reads it |
| Folded allow/block partition | in memory | `firewall.Partition` | one folding function, used by the firewall renderer **and** Stage A1 |
| Service (L7 and L4) | `services` (`kind` http/https/h2c/tcp/udp) | `services.Service` | one model; only HTTP kinds own a root rule set |
| Origin and locking | `origin` (invariant 9) + `locked` flags on sets, elements, rule sets, rule entries | shared enums | one permission check: what may be edited is derived from `kind` + `origin` + `locked` |
| Address references from consumers | `ip_set_id` columns (§0.3) | `ipsets.Ref` | resolution via the compiler only |

### 0.2 Layer-specific — compiled from the shared objects

| Layer | Compiled form | From |
| --- | --- | --- |
| Forward auth (L7) | interval lists + GeoIP predicates per set; flattened rule list per service | `ipsets.Compiled`, `rules.Flatten` |
| Firewall (L3/L4) | `allow`/`block`/`host_access` ranges in `table inet hashi`; `dyn` elements | `firewall.Partition`, dynamic set entries |
| Targets (DNS values, upstreams, connections) | ordered address lists per family | `ipsets.Compiled` (address-set capability required) |

### 0.3 Deliberate deviations (and why)

| Where | Concept says | Implementation | Reason |
| --- | --- | --- | --- |
| Stage A | Global rules applying to `all` / `dynamic-challenge` | Engine code; shown as locked entries in traces and flattened views | A2 is a windowed counter with side effects, not match → action; Stage A must not be removable or reorderable |
| Kernel sets (`allow`, `block`, `dyn`, `staging`, meter sets) | "the firewall's IP sets" | Compiled artifacts, not `ip_sets` rows | They are outputs of folding; `staging` and meter state exist only in the kernel |
| "Used by" an IP set | one relation "consumer uses set" | A typed `ip_set_id` FK column per consumer (`ON DELETE RESTRICT`) + one `ip_set_consumers` view (UNION) | A polymorphic `(consumer_type, consumer_id)` table loses referential integrity |
| Engine data on dynamic entries | an IP in a set | `ip_set_elements` row + `dynamic_entry_state` 1:1 + `dynamic_entry_hosts` | failure count, source, per-host apply state are engine-only and churn often |
| Ban counter | property of an address | `ban_counters` keyed by address (/64 for IPv6), not an element | it outlives set membership (persists after the ban ends, decays) |
| Thresholds, timers, `T_ban` | "settings" | typed `settings.Security` category (S1 framework) | rules vs settings is a principle of the model (06 §1) |
| Pulse agents, hosts, DNS names | sources of addresses | own entities, referenced by items | they have lifecycles of their own; items never copy their addresses |
| Telemetry | — | buckets/events tables | append-only facts, not domain objects |

### 0.4 Go packages

`internal/ipsets` (types, store, cycle check, compiler, reactivity graph) · `internal/rules` (rule sets, flatten,
evaluate, trace) · `internal/firewall` (folding → `Partition`, nft renderer, host link, rescue) · `internal/edge`
(forward auth: Stage A engine + Stage B via `rules`, SSO, CAPTCHA) · `internal/security` (dynamic entries, ban counters,
feeds, telemetry, GeoIP). Dependency direction: `edge`, `firewall`, `dns`, `traefik`, `services` → `rules` → `ipsets`;
`ipsets` depends on nothing above it (reads Pulse/host/DNS/GeoIP through small interfaces).

## 1. Common columns

Every table below has `id`, `created_at`, `updated_at` unless stated. `origin` ∈ `system` | `managed` | `imported` |
`user` | `feed` | `engine` (the last two only on `ip_set_elements`). `locked` booleans mean "not editable by the user".

## 2. IP sets

### 2.1 `ip_sets`

`slug` (unique), `name`, `description`, `kind` (`user` | `system` | `feed` | `dynamic`), `origin`, `system_key`
(`all`, `hashi_hosts`, `private_networks`, `dynamic_challenge`, `dynamic_blocked`; unique when set), `compiled_hash`,
`resolution_status` (`ok` | `partial` | `empty`), `resolution_problems_json`, `compiled_at`.

### 2.2 `ip_set_elements`

`ip_set_id`, `position`, `type` (`ip` | `cidr` | `pulse` | `host` | `dns` | `geo` | `set`), `origin`, `locked`,
`address` (text, normalized; `ip`), `prefix` (text, normalized; `cidr`), `family` (4/6; `ip`, `cidr`),
`pulse_agent_id` + `scope` + `selector` (`pulse`), `host_id` + `scope` (`host`), `fqdn` (`dns`),
`geo_kind` (`country` | `region` | `asn`) + `geo_value` (`geo`), `child_set_id` (`set`, FK `ON DELETE RESTRICT`),
`note`, `expires_at`, `created_by`.

`CHECK` per type: exactly the columns of that type are non-null. Indexes: `(ip_set_id, position)`, `(ip_set_id, address)`,
`(ip_set_id, prefix)`, `(child_set_id)`, `(pulse_agent_id)`, `(host_id)`, `(expires_at)`.

Feed refreshes update a feed set's elements by diff (insert new, delete gone) in one transaction.

### 2.3 Engine side tables

`dynamic_entry_state` (1:1 with an element of a `dynamic` set): `element_id`, `source` (`l7_rate` | `l7_waf` |
`l7_challenge_failed` | `l4_meter` | `manual`), `meter` (for `l4_meter`), `firewall_host_id` (origin host for `l4_meter`),
`service_id`, `reason`, `failure_count` (challenge entries), `removed_at`, `removed_reason`.

`dynamic_entry_hosts`: `element_id`, `firewall_host_id`, `state` (`pending` | `applied` | `removed` | `failed`),
`applied_at`, `error`.

`ban_counters`: `address` (IPv4 or IPv6 /64, unique), `count`, `last_ban_at`, `last_ban_ends_at`.

### 2.4 Feeds

`ip_set_feeds` (1:1 with a `feed` set): `url`, `source_kind` (`recommended` | `custom`), `format`,
`format_options_json`, `refresh_interval_seconds`, `last_fetch_started_at`, `last_success_at`, `last_error`,
`last_http_status`, `last_etag`, `last_modified`, `entry_count`, `rejected_count`.
`ip_set_feed_runs`: `ip_set_id`, `started_at`, `completed_at`, `status`, `http_status`, `parsed`, `added`, `removed`,
`unchanged`, `rejected`, `error`, `content_hash`.

### 2.5 Consumers (`ip_set_id` columns)

| Table.column | Position |
| --- | --- |
| `firewall_bindings.ip_set_id` (`position` = `allow` \| `block`, `order`) | match |
| `firewall_host_access.ip_set_id` (`firewall_host_id`) | match |
| `rule_entries.applies_to_set_id` | match |
| `dns_records.value_set_id` (A/AAAA records with set-based values) | target |
| `service_targets.address_set_id` | target |
| `connections.target_set_id` (SSH and API targets; replaces `connection_targets`) | target |
| `pulse_agents` internal DNS uses the agent directly (03 §9) | — |

`ip_set_consumers` view: `ip_set_id`, `consumer_type`, `consumer_id`, `position` — the UNION of the columns above plus
`ip_set_elements.child_set_id` (parent sets). Used by "Used by" lists and delete checks.

## 3. Rule sets

`rule_sets`: `slug`, `name`, `description`, `kind` (`user` | `global` | `service_root`), `origin`, `service_id` (only
`service_root`, unique), `flattened_hash`.

`rule_entries`: `rule_set_id`, `priority` (unique per `rule_set_id`), `entry_type` (`item` | `set`), `child_rule_set_id`
(`set`; FK `ON DELETE RESTRICT`; never a `service_root`), `applies_to_set_id` (`item`), `match_type` (`exact` | `prefix` |
`regex`, null = prefix `/`), `pattern`, `action` (`none` | `deny` | `pass_captcha` | `pass_authentication`),
`auth_provider_id`, `auth_groups_json`, `origin`, `locked`, `is_fallback` (exactly one per `service_root`, always the
highest priority number), `enabled`, `note`.

## 4. Services (security columns)

`services.root_rule_set_id` (HTTP kinds), `service_waf_modes`: `service_id`, `match_type`, `pattern` (null = service
root), `mode` (`enabled` | `headers_only` | `disabled`), `exclusions_json` (CRS rule IDs).

## 5. Edge, CAPTCHA, GeoIP

`oidc_providers` (issuer, client ID, secret ref, scopes, claim mapping, `is_default`), `edge_sessions`,
`captcha_settings`, `challenge_attempts` (address, scope `global`/`service`, service, result, at), `geoip_databases`
(edition, version, checksum, downloaded_at). Clearance cookies are stateless (signed).

## 6. Telemetry

`security_events`: `occurred_at`, `address` (normalized; /64 for IPv6 engine events), `service_id`, `firewall_host_id`,
`traefik_connection_id`, `event_type`, `severity`, `decision`, `source`, `reason`, `request_method`, `request_path`,
`status_code`, `user_agent_hash`, `request_id`, `suppressed`, `metadata_json`. Indexes on (address, time), (service, time),
(event_type, time), (severity, time).

`security_buckets_1m`: `bucket_start`, `service_id`, `root_domain`, `traefik_connection_id`, `country`, `asn`, `decision`,
`status_class`, `path_prefix`, `request_count`, `denied_count`, `challenged_count`, `challenge_failed_count`,
`banned_count`. No address (volume bound, 01-platform.md §4.3).

`security_ip_top_1m`: `bucket_start`, `address`, `service_id`, request/denied/challenged counts; index (address, time).

## 7. Firewall state

`firewall_bindings`, `firewall_host_access` (§2.5); `firewall_host_sets`: `firewall_host_id`, kernel set name,
`last_applied_hash`, `element_count`, `applied_at` (structural sets only; dynamic entries are tracked per entry in
`dynamic_entry_hosts`); `firewall_host_link`: `firewall_host_id`, `state`, `last_poll_at`, `last_push_at`, `lag_ms`, `error`.
