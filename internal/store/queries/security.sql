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
