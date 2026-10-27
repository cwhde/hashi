# 06a — Security data model (detail for 06-edge-security.md)

> Carried over verbatim from the V2 specification. Column names use the old `_utc` suffix; in the new schema they are
> integer Unix-millisecond columns ending in `_at` (01-platform.md §4.1). Everything else applies as written.

## Security subjects, states, manual entries, events, buckets

Add or extend these tables.

#### `security_subjects`

Represents normalized subjects.

Fields:

* `id`.
* `subject_type`: `ip`, `cidr`, `asn`, `country`, `region`, `session`, `composite`.
* `subject_value`.
* `normalized_value`.
* `first_seen_at_utc`.
* `last_seen_at_utc`.
* `last_country`.
* `last_region`.
* `last_asn`.
* `last_as_org`.
* `current_state`.
* `metadata_json`.

Unique index:

* `(subject_type, normalized_value)`.

#### `security_subject_states`

Tracks active state.

Fields:

* `id`.
* `security_subject_id`.
* `challenge_required`.
* `challenge_required_since_utc`.
* `challenge_reason`.
* `challenge_resource_id`.
* `challenge_attempts`.
* `requests_while_challenged`.
* `failed_challenge_count`.
* `successful_challenge_count`.
* `last_challenge_solved_at_utc`.
* `soft_blocked_until_utc`.
* `firewall_blocked_until_utc`.
* `manual_allow_active`.
* `manual_block_active`.
* `last_escalation_reason`.
* `last_escalation_at_utc`.
* `updated_at_utc`.

#### `manual_security_entries`

Manual allow/block list.

Fields:

* `id`.
* `subject_type`.
* `subject_value`.
* `normalized_value`.
* `entry_type`: `allow` or `block`.
* `scope_type`: `global`, `resource`, `root_domain`, `traefik_connection`, `firewall_host`.
* `scope_id`.
* `reason`.
* `created_by_admin_id`.
* `created_at_utc`.
* `expires_at_utc`.
* `is_permanent`.
* `bypass_blocking`.
* `bypass_adaptive_escalation`.
* `bypass_rate_limit`.
* `bypass_challenge`.
* `bypass_sso`.
* `enabled`.
* `last_hit_at_utc`.

Constraints:

* `bypass_sso` and `bypass_challenge` must default to false.
* If `entry_type = block`, bypass flags must be false.

#### `security_events`

Extend existing events if already present.

Required fields:

* `id`.
* `occurred_at_utc`.
* `subject_type`.
* `subject_value`.
* `normalized_subject_value`.
* `resource_id`.
* `connection_id`.
* `event_type`.
* `severity`.
* `decision`.
* `source`.
* `reason`.
* `request_method`.
* `request_path`.
* `status_code`.
* `user_agent_hash`.
* `request_id`.
* `metadata_json`.

Indexes:

* `(normalized_subject_value, occurred_at_utc desc)`.
* `(resource_id, occurred_at_utc desc)`.
* `(event_type, occurred_at_utc desc)`.
* `(severity, occurred_at_utc desc)`.

#### `security_request_buckets`

Tracks aggregate request pressure.

Fields:

* `id`.
* `bucket_start_utc`.
* `bucket_size_seconds`.
* `subject_type`.
* `normalized_subject_value`.
* `resource_id`.
* `root_domain`.
* `country`.
* `region`.
* `asn`.
* `method`.
* `path_prefix`.
* `status_class`.
* `request_count`.
* `blocked_count`.
* `challenged_count`.
* `challenge_ignored_count`.
* `failed_challenge_count`.

Indexes must support quick lookup by subject and recent time window.

## Blocklists

#### `blocklist_sources`

Fields:

* `id`.
* `name`.
* `description`.
* `source_url`.
* `source_kind`: `recommended` or `custom`.
* `format`.
* `format_options_json`.
* `enabled`.
* `default_enforcement_mode`.
* `fetch_interval_seconds`.
* `last_fetch_started_at_utc`.
* `last_fetch_completed_at_utc`.
* `last_success_at_utc`.
* `last_error`.
* `last_http_status`.
* `last_etag`.
* `last_modified`.
* `entry_count`.
* `rejected_count`.
* `created_at_utc`.
* `updated_at_utc`.

#### `blocklist_entries`

Fields:

* `id`.
* `blocklist_source_id`.
* `subject_type`: `ip` or `cidr`.
* `subject_value`.
* `normalized_value`.
* `first_seen_at_utc`.
* `last_seen_at_utc`.
* `enabled`.
* `enforcement_mode`: `middleware`, `firewall`, `both`.
* `metadata_json`.

Indexes:

* `(blocklist_source_id, normalized_value)`.
* `(normalized_value)`.
* `(subject_type, normalized_value)`.

#### `blocklist_fetch_runs`

Fields:

* `id`.
* `blocklist_source_id`.
* `started_at_utc`.
* `completed_at_utc`.
* `status`.
* `http_status`.
* `entries_parsed`.
* `entries_added`.
* `entries_removed`.
* `entries_unchanged`.
* `entries_rejected`.
* `error`.
* `content_hash`.

## 15. Firewall Integration

Only IP and CIDR subjects may be firewall-enforced by default.

Firewall sync must:

* Use Hashi-owned chains/sets only.
* Never flush unrelated firewall state.
* Preview changes.
* Apply atomically where possible.
* Roll back or report partial failure.
* Track applied hosts.
* Track last applied hash.

Firewall block entries must record:

* Target host.
* Applied state.
* Last apply result.
* Last apply error.
* Last hit time, where counters are available.

ASN/country/region blocks remain forward-auth only by default.
