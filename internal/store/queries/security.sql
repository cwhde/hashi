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
VALUES (?, ?, '', '', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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

-- Security dashboard (06 section 13). Long periods read whole rolled hours from the hourly tables before until and
-- the minutes after; the handler adds both (bug 99). Short periods read minutes only.
-- name: SecurityDecisionSeries :many
SELECT CAST(bucket_start / sqlc.arg(step) * sqlc.arg(step) AS INTEGER) AS at, decision,
    CAST(SUM(request_count) AS INTEGER) AS requests
FROM security_buckets_1m
WHERE bucket_start >= sqlc.arg(since) AND decision != 'response'
GROUP BY 1, 2 ORDER BY 1;

-- name: SecurityHourDecisionSeries :many
SELECT CAST(bucket_start / sqlc.arg(step) * sqlc.arg(step) AS INTEGER) AS at, decision,
    CAST(SUM(request_count) AS INTEGER) AS requests
FROM security_buckets_1h
WHERE bucket_start >= sqlc.arg(since) AND bucket_start < sqlc.arg(until) AND decision != 'response'
GROUP BY 1, 2 ORDER BY 1;

-- name: SecurityEventSeries :many
SELECT CAST(occurred_at / sqlc.arg(step) * sqlc.arg(step) AS INTEGER) AS at, event_type, source,
    CAST(COUNT(*) + SUM(suppressed) AS INTEGER) AS events
FROM security_events
WHERE occurred_at >= sqlc.arg(since)
    AND event_type IN ('waf_match', 'ban_created', 'challenge_entered', 'challenge_solved')
GROUP BY 1, 2, 3 ORDER BY 1;

-- The 100 most refused addresses of each table; the handler adds them up and keeps 10.
-- name: SecurityTopRefused :many
SELECT address, CAST(SUM(request_count) AS INTEGER) AS requests, CAST(SUM(denied_count) AS INTEGER) AS denied,
    CAST(SUM(challenged_count) AS INTEGER) AS challenged
FROM security_ip_top_1m
WHERE bucket_start >= sqlc.arg(since)
-- "+address": group without the address index, so SQLite reads only the period through the primary key instead of
-- the whole table (bug 99).
GROUP BY +address HAVING SUM(denied_count) + SUM(challenged_count) > 0
ORDER BY SUM(denied_count) + SUM(challenged_count) DESC, SUM(request_count) DESC
LIMIT 100;

-- name: SecurityHourTopRefused :many
SELECT address, CAST(SUM(request_count) AS INTEGER) AS requests, CAST(SUM(denied_count) AS INTEGER) AS denied,
    CAST(SUM(challenged_count) AS INTEGER) AS challenged
FROM security_ip_top_1h
WHERE bucket_start >= sqlc.arg(since) AND bucket_start < sqlc.arg(until)
GROUP BY address HAVING SUM(denied_count) + SUM(challenged_count) > 0
ORDER BY SUM(denied_count) + SUM(challenged_count) DESC, SUM(request_count) DESC
LIMIT 100;

-- name: SecurityRefusedByCountry :many
SELECT country, CAST(SUM(denied_count + challenged_count) AS INTEGER) AS refused
FROM security_buckets_1m
WHERE bucket_start >= sqlc.arg(since) AND decision != 'response'
GROUP BY country HAVING SUM(denied_count + challenged_count) > 0;

-- name: SecurityHourRefusedByCountry :many
SELECT country, CAST(SUM(denied_count + challenged_count) AS INTEGER) AS refused
FROM security_buckets_1h
WHERE bucket_start >= sqlc.arg(since) AND bucket_start < sqlc.arg(until) AND decision != 'response'
GROUP BY country HAVING SUM(denied_count + challenged_count) > 0;

-- name: SecurityRefusedByService :many
SELECT b.service_id, COALESCE(s.name, '') AS name, CAST(SUM(b.denied_count + b.challenged_count) AS INTEGER) AS refused
FROM security_buckets_1m b LEFT JOIN services s ON s.id = b.service_id
WHERE b.bucket_start >= sqlc.arg(since) AND b.decision != 'response'
GROUP BY b.service_id HAVING SUM(b.denied_count + b.challenged_count) > 0;

-- name: SecurityHourRefusedByService :many
SELECT b.service_id, COALESCE(s.name, '') AS name, CAST(SUM(b.denied_count + b.challenged_count) AS INTEGER) AS refused
FROM security_buckets_1h b LEFT JOIN services s ON s.id = b.service_id
WHERE b.bucket_start >= sqlc.arg(since) AND b.bucket_start < sqlc.arg(until) AND b.decision != 'response'
GROUP BY b.service_id HAVING SUM(b.denied_count + b.challenged_count) > 0;

-- Hourly rollup of the security minutes (01 section 4.3). Each run replaces the last few hours, so access-log lines
-- read late still count.
-- name: LastSecurityHour :one
SELECT CAST(COALESCE(MAX(bucket_start), 0) AS INTEGER) FROM security_buckets_1h;

-- name: NextSecurityMinute :one
SELECT CAST(COALESCE(MIN(bucket_start), 0) AS INTEGER) FROM security_buckets_1m WHERE bucket_start >= ?;

-- name: ClearSecurityHours :exec
DELETE FROM security_buckets_1h WHERE bucket_start >= sqlc.arg(from_at) AND bucket_start < sqlc.arg(to_at);

-- name: RollupSecurityHours :exec
INSERT INTO security_buckets_1h (bucket_start, service_id, root_domain, traefik_connection_id, country, asn, decision,
    status_class, request_count, denied_count, challenged_count, challenge_failed_count, banned_count)
SELECT m.bucket_start / 3600000 * 3600000, m.service_id, m.root_domain, m.traefik_connection_id, m.country, m.asn,
    m.decision, m.status_class, SUM(m.request_count), SUM(m.denied_count), SUM(m.challenged_count),
    SUM(m.challenge_failed_count), SUM(m.banned_count)
FROM security_buckets_1m m
WHERE m.bucket_start >= sqlc.arg(from_at) AND m.bucket_start < sqlc.arg(to_at)
GROUP BY 1, 2, 3, 4, 5, 6, 7, 8;

-- name: ClearSecurityTopHours :exec
DELETE FROM security_ip_top_1h WHERE bucket_start >= sqlc.arg(from_at) AND bucket_start < sqlc.arg(to_at);

-- name: RollupSecurityTopHours :exec
INSERT INTO security_ip_top_1h (bucket_start, address, service_id, request_count, denied_count, challenged_count)
SELECT r.hour, r.address, r.service_id, r.requests, r.denied, r.challenged FROM (
    SELECT m.bucket_start / 3600000 * 3600000 AS hour, m.address, m.service_id, SUM(m.request_count) AS requests,
        SUM(m.denied_count) AS denied, SUM(m.challenged_count) AS challenged,
        ROW_NUMBER() OVER (PARTITION BY m.bucket_start / 3600000
            ORDER BY SUM(m.denied_count) + SUM(m.challenged_count) DESC, SUM(m.request_count) DESC) AS place
    FROM security_ip_top_1m m
    WHERE m.bucket_start >= sqlc.arg(from_at) AND m.bucket_start < sqlc.arg(to_at)
    GROUP BY 1, 2, 3
) r WHERE r.place <= 500;

-- Retention (01 section 4.3): each batch ends before its 5000th row, so one delete stays small.
-- name: SecurityMinuteBatchEnd :one
SELECT bucket_start FROM security_buckets_1m WHERE bucket_start < ? ORDER BY bucket_start LIMIT 1 OFFSET 4999;

-- name: DeleteOldSecurityMinutes :execrows
DELETE FROM security_buckets_1m WHERE bucket_start < ?;

-- name: SecurityTopMinuteBatchEnd :one
SELECT bucket_start FROM security_ip_top_1m WHERE bucket_start < ? ORDER BY bucket_start LIMIT 1 OFFSET 4999;

-- name: DeleteOldSecurityTopMinutes :execrows
DELETE FROM security_ip_top_1m WHERE bucket_start < ?;

-- name: SecurityHourBatchEnd :one
SELECT bucket_start FROM security_buckets_1h WHERE bucket_start < ? ORDER BY bucket_start LIMIT 1 OFFSET 4999;

-- name: DeleteOldSecurityHours :execrows
DELETE FROM security_buckets_1h WHERE bucket_start < ?;

-- name: SecurityTopHourBatchEnd :one
SELECT bucket_start FROM security_ip_top_1h WHERE bucket_start < ? ORDER BY bucket_start LIMIT 1 OFFSET 4999;

-- name: DeleteOldSecurityTopHours :execrows
DELETE FROM security_ip_top_1h WHERE bucket_start < ?;

-- name: SecurityEventBatchEnd :one
SELECT occurred_at FROM security_events WHERE occurred_at < ? ORDER BY occurred_at LIMIT 1 OFFSET 4999;

-- name: DeleteOldSecurityEvents :execrows
DELETE FROM security_events WHERE occurred_at < ?;

-- name: SecurityMeterBans :many
SELECT COALESCE(e.firewall_host_id, '') AS host_id, COALESCE(h.name, '') AS host, CAST(COUNT(*) AS INTEGER) AS bans
FROM security_events e LEFT JOIN firewall_hosts h ON h.id = e.firewall_host_id
WHERE e.occurred_at >= sqlc.arg(since) AND e.event_type = 'ban_created' AND e.source = 'l4_meter'
GROUP BY 1, 2 ORDER BY 2;

-- name: ListRecentSecurityEvents :many
SELECT e.occurred_at, e.address, e.event_type, e.severity, e.source, e.reason, e.request_method, e.request_path,
    COALESCE(e.service_id, '') AS service_id, COALESCE(s.name, '') AS service, COALESCE(h.name, '') AS host,
    e.suppressed, e.metadata_json
FROM security_events e
    LEFT JOIN services s ON s.id = e.service_id
    LEFT JOIN firewall_hosts h ON h.id = e.firewall_host_id
WHERE e.occurred_at >= sqlc.arg(since)
ORDER BY e.occurred_at DESC LIMIT 30;

-- Events of one address for the address page (05a section 6), shaped like the dashboard list.
-- name: ListAddressSecurityEvents :many
SELECT e.occurred_at, e.address, e.event_type, e.severity, e.source, e.reason, e.request_method, e.request_path,
    COALESCE(e.service_id, '') AS service_id, COALESCE(s.name, '') AS service, COALESCE(h.name, '') AS host,
    e.suppressed, e.metadata_json
FROM security_events e
    LEFT JOIN services s ON s.id = e.service_id
    LEFT JOIN firewall_hosts h ON h.id = e.firewall_host_id
WHERE e.address = sqlc.arg(address)
ORDER BY e.occurred_at DESC LIMIT 20;

-- Counters view (06 section 13.1): the cheap filters here; state, minimums and decay are computed in Go. An empty
-- source or country, and asn or since 0, mean any.
-- name: ListAddressCounters :many
SELECT * FROM address_counters
WHERE (CAST(sqlc.arg(source) AS TEXT) = '' OR last_source = CAST(sqlc.arg(source) AS TEXT))
    AND (CAST(sqlc.arg(country) AS TEXT) = '' OR country = CAST(sqlc.arg(country) AS TEXT))
    AND (CAST(sqlc.arg(asn) AS INTEGER) = 0 OR asn = CAST(sqlc.arg(asn) AS INTEGER))
    AND (CAST(sqlc.arg(since) AS INTEGER) = 0 OR COALESCE(last_ban_at, 0) >= CAST(sqlc.arg(since) AS INTEGER) OR COALESCE(last_challenge_at, 0) >= CAST(sqlc.arg(since) AS INTEGER))
ORDER BY address;
