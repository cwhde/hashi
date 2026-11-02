-- name: ListDynamicEntries :many
SELECT e.id, e.ip_set_id, e.address, e.prefix, e.expires_at, e.created_at, d.source, d.meter, d.firewall_host_id,
    d.service_id, d.reason, d.failure_count
FROM ip_set_elements e JOIN dynamic_entry_state d ON d.element_id = e.id
WHERE e.ip_set_id IN ('sys-dynamic-challenge', 'sys-dynamic-blocked') AND d.removed_at IS NULL
ORDER BY e.created_at;

-- name: InsertDynamicEntryState :exec
INSERT INTO dynamic_entry_state (element_id, source, meter, firewall_host_id, service_id, reason, failure_count)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: SetDynamicEntryFailures :exec
UPDATE dynamic_entry_state SET failure_count = ? WHERE element_id = ?;

-- name: DeleteDynamicElement :exec
DELETE FROM ip_set_elements WHERE id = ? AND ip_set_id IN ('sys-dynamic-challenge', 'sys-dynamic-blocked');

-- name: GetAddressCounters :one
SELECT * FROM address_counters WHERE address = ?;

-- name: UpsertAddressCounters :exec
INSERT INTO address_counters (address, ban_count, ban_quiet_since, challenge_count, challenge_quiet_since, last_ban_at,
    last_ban_ends_at, last_challenge_at, last_source, country, asn, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (address) DO UPDATE SET ban_count = excluded.ban_count, ban_quiet_since = excluded.ban_quiet_since,
    challenge_count = excluded.challenge_count, challenge_quiet_since = excluded.challenge_quiet_since,
    last_ban_at = excluded.last_ban_at, last_ban_ends_at = excluded.last_ban_ends_at,
    last_challenge_at = excluded.last_challenge_at, last_source = excluded.last_source, country = excluded.country,
    asn = excluded.asn, updated_at = excluded.updated_at;

-- name: InsertDynamicEntryHost :exec
INSERT INTO dynamic_entry_hosts (element_id, firewall_host_id, state, applied_at, error) VALUES (?, ?, ?, ?, ?)
ON CONFLICT (element_id, firewall_host_id) DO UPDATE SET state = excluded.state, applied_at = excluded.applied_at,
    error = excluded.error;

-- name: ListDynamicEntryHosts :many
SELECT * FROM dynamic_entry_hosts WHERE firewall_host_id = ?;

-- name: ListManagedFirewallHostIDs :many
SELECT id FROM firewall_hosts WHERE edge_mode = 'managed' ORDER BY id;

-- name: InsertSecurityEvent :exec
INSERT INTO security_events (id, occurred_at, address, service_id, firewall_host_id, traefik_connection_id, event_type,
    severity, decision, source, reason, request_method, request_path, status_code, user_agent_hash, request_id,
    suppressed, metadata_json)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: AddSecurityEventSuppressed :exec
UPDATE security_events SET suppressed = suppressed + ? WHERE id = ?;

-- name: ListSecurityEventsForAddress :many
SELECT * FROM security_events WHERE address = ? ORDER BY occurred_at DESC LIMIT ?;

-- name: PutFirewallHostLink :exec
INSERT INTO firewall_host_link (firewall_host_id, state, last_poll_at, last_push_at, lag_ms, error, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (firewall_host_id) DO UPDATE SET state = excluded.state,
    last_poll_at = COALESCE(excluded.last_poll_at, firewall_host_link.last_poll_at),
    last_push_at = COALESCE(excluded.last_push_at, firewall_host_link.last_push_at), lag_ms = excluded.lag_ms,
    error = excluded.error, updated_at = excluded.updated_at;

-- name: GetFirewallHostLink :one
SELECT * FROM firewall_host_link WHERE firewall_host_id = ?;

-- name: GetAccessLogCursor :one
SELECT * FROM access_log_cursors WHERE traefik_connection_id = ? AND file = ?;

-- name: PutAccessLogCursor :exec
INSERT INTO access_log_cursors (traefik_connection_id, file, inode, offset, lag_ms, last_read_at, error, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (traefik_connection_id, file) DO UPDATE SET inode = excluded.inode, offset = excluded.offset,
    lag_ms = excluded.lag_ms, last_read_at = COALESCE(excluded.last_read_at, access_log_cursors.last_read_at),
    error = excluded.error, updated_at = excluded.updated_at;

-- name: AddSecurityBucket :exec
INSERT INTO security_buckets_1m (bucket_start, service_id, root_domain, traefik_connection_id, country, asn, decision,
    status_class, path_prefix, request_count, denied_count, challenged_count, challenge_failed_count, banned_count)
VALUES (?, ?, '', '', ?, ?, ?, '', ?, ?, ?, ?, ?, ?)
ON CONFLICT DO UPDATE SET request_count = request_count + excluded.request_count,
    denied_count = denied_count + excluded.denied_count, challenged_count = challenged_count + excluded.challenged_count,
    challenge_failed_count = challenge_failed_count + excluded.challenge_failed_count,
    banned_count = banned_count + excluded.banned_count;

-- name: AddSecurityTop :exec
INSERT INTO security_ip_top_1m (bucket_start, address, service_id, request_count, denied_count, challenged_count)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT DO UPDATE SET request_count = request_count + excluded.request_count,
    denied_count = denied_count + excluded.denied_count, challenged_count = challenged_count + excluded.challenged_count;

-- name: ListSecurityBuckets :many
SELECT * FROM security_buckets_1m WHERE bucket_start >= ? ORDER BY bucket_start;
