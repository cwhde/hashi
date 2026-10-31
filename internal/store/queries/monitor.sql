-- name: ListMonitorEndpoints :many
SELECT * FROM monitor_endpoints ORDER BY group_name, name;

-- name: GetMonitorEndpoint :one
SELECT * FROM monitor_endpoints WHERE id = ?;

-- name: InsertMonitorEndpoint :exec
INSERT INTO monitor_endpoints (id, source_type, source_id, name, group_name, interval_seconds, timeout_seconds, degraded_ms, public, state_since, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: RenameMonitorEndpoint :exec
UPDATE monitor_endpoints SET name = ?, updated_at = ? WHERE id = ?;

-- name: UpdateMonitorEndpointSettings :exec
UPDATE monitor_endpoints
SET group_name = ?, paused = ?, interval_seconds = ?, timeout_seconds = ?, degraded_ms = ?, public = ?, route_provider_id = ?, updated_at = ?
WHERE id = ?;

-- name: SetMonitorEndpointResult :exec
UPDATE monitor_endpoints
SET state = ?, state_since = ?, good_rounds = ?, bad_rounds = ?, last_check_at = ?, last_latency_ms = ?
WHERE id = ?;

-- name: DeleteMonitorEndpoint :exec
DELETE FROM monitor_endpoints WHERE id = ?;

-- name: ListMonitorChecks :many
SELECT * FROM monitor_checks WHERE endpoint_id = ? ORDER BY type, role, target;

-- name: InsertMonitorCheck :exec
INSERT INTO monitor_checks (id, endpoint_id, type, role, target, required, code_min, code_max, skip_verify, probe, expect)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: DeleteMonitorChecks :exec
DELETE FROM monitor_checks WHERE endpoint_id = ?;

-- name: InsertMonitorSample :exec
INSERT OR REPLACE INTO monitor_samples (endpoint_id, check_id, at, verdict, ok, latency_ms, detail) VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: ListMonitorSamplesSince :many
SELECT * FROM monitor_samples WHERE endpoint_id = ? AND check_id = ? AND at >= ? ORDER BY at;

-- name: ListMonitorSamplesBetween :many
SELECT * FROM monitor_samples WHERE at >= ? AND at < ? ORDER BY endpoint_id, check_id, at;

-- name: ListRoundsSince :many
SELECT endpoint_id, at, verdict FROM monitor_samples WHERE check_id = '' AND at >= ? ORDER BY endpoint_id, at;

-- name: UpsertMonitorRollup :exec
INSERT OR REPLACE INTO monitor_rollups (endpoint_id, check_id, resolution, bucket, count, ok_count, min_ms, max_ms, avg_ms, p95_ms)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: LastRollupBucket :one
SELECT CAST(COALESCE(MAX(bucket), 0) AS INTEGER) FROM monitor_rollups WHERE resolution = ?;

-- name: ListMonitorRollups :many
SELECT * FROM monitor_rollups WHERE endpoint_id = ? AND check_id = ? AND resolution = ? AND bucket >= ? ORDER BY bucket;

-- name: UptimeSince :one
SELECT CAST(COALESCE(SUM(ok), 0) AS INTEGER) AS ok_count, COUNT(*) AS total
FROM monitor_samples WHERE endpoint_id = ? AND check_id = '' AND at >= ?;

-- name: InsertMonitorEvent :exec
INSERT INTO monitor_events (id, endpoint_id, at, from_state, to_state, message) VALUES (?, ?, ?, ?, ?, ?);

-- name: ListMonitorEvents :many
SELECT * FROM monitor_events WHERE endpoint_id = ? ORDER BY at DESC LIMIT ?;

-- Retention (01 section 4.3): the batch's end is the 5000th oldest row; deleting below it stays under the batch size.

-- name: SampleBatchEnd :one
SELECT at FROM monitor_samples WHERE at < ? ORDER BY at LIMIT 1 OFFSET 4999;

-- name: DeleteOldMonitorSamples :execrows
DELETE FROM monitor_samples WHERE at < ?;

-- name: RollupBatchEnd :one
SELECT bucket FROM monitor_rollups WHERE resolution = ? AND bucket < ? ORDER BY bucket LIMIT 1 OFFSET 4999;

-- name: DeleteOldMonitorRollups :execrows
DELETE FROM monitor_rollups WHERE resolution = ? AND bucket < ?;

-- name: DeleteOldJobRuns :execrows
DELETE FROM job_runs WHERE kind = ? AND started_at < ?;

-- name: ListNotificationProviders :many
SELECT * FROM notification_providers ORDER BY name;

-- name: GetNotificationProvider :one
SELECT * FROM notification_providers WHERE id = ?;

-- name: InsertNotificationProvider :exec
INSERT INTO notification_providers (id, type, name, config_json, secret_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: UpdateNotificationProvider :exec
UPDATE notification_providers SET name = ?, config_json = ?, secret_id = ?, updated_at = ? WHERE id = ?;

-- name: DeleteNotificationProvider :exec
DELETE FROM notification_providers WHERE id = ?;

-- name: GetNotificationRoute :one
SELECT * FROM notification_route WHERE id = 1;

-- name: PutNotificationRoute :exec
UPDATE notification_route SET provider_id = ?, min_severity = ?, cooldown_seconds = ?, recovery = ?, updated_at = ? WHERE id = 1;

-- name: InsertNotificationDelivery :exec
INSERT INTO notification_deliveries (id, provider_id, endpoint_id, kind, subject, body, next_attempt_at, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: DueNotificationDeliveries :many
SELECT * FROM notification_deliveries WHERE status = 'pending' AND next_attempt_at <= ? ORDER BY created_at LIMIT 50;

-- name: FinishNotificationDelivery :exec
UPDATE notification_deliveries SET status = ?, attempts = ?, next_attempt_at = ?, last_error = ?, delivered_at = ? WHERE id = ?;

-- name: ListNotificationDeliveries :many
SELECT * FROM notification_deliveries ORDER BY created_at DESC LIMIT ?;

-- name: LastAlertAt :one
SELECT CAST(COALESCE(MAX(next_attempt_at), 0) AS INTEGER) FROM notification_deliveries WHERE endpoint_id = ? AND kind = 'alert';

-- name: PendingAlerts :one
SELECT COUNT(*) FROM notification_deliveries WHERE endpoint_id = ? AND kind = 'alert' AND status = 'pending';

-- name: DeletePendingAlerts :execrows
DELETE FROM notification_deliveries WHERE endpoint_id = ? AND kind = 'alert' AND status = 'pending';

-- name: DeleteNotificationDelivery :exec
DELETE FROM notification_deliveries WHERE id = ?;

-- name: ListMonitoredDNSRecords :many
SELECT r.id, r.name, r.type, r.display_name, z.name AS zone_name
FROM dns_records r JOIN dns_zones z ON z.id = r.zone_id WHERE r.monitoring = 1 ORDER BY z.name, r.name;

-- name: ListDashboardDNSRecords :many
SELECT r.id, r.name, r.display_name, z.name AS zone_name
FROM dns_records r JOIN dns_zones z ON z.id = r.zone_id WHERE r.dashboard = 1 ORDER BY r.display_name;

-- name: SetServiceDashboard :execrows
UPDATE services SET dashboard_visible = ?, updated_at = ? WHERE id = ?;
