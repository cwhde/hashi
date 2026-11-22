-- name: GetPublicLayout :one
SELECT version, seeded FROM public_layout WHERE id = 1;

-- name: SetPublicLayoutVersion :exec
UPDATE public_layout SET version = ?, seeded = 1 WHERE id = 1;

-- name: ListPublicGroups :many
SELECT id, name, position FROM public_groups ORDER BY position;

-- name: ListPublicEntries :many
SELECT subject_type, subject_id, group_id, position FROM public_entries ORDER BY position;

-- name: DeletePublicEntries :exec
DELETE FROM public_entries;

-- name: DeletePublicGroups :exec
DELETE FROM public_groups;

-- name: InsertPublicGroup :exec
INSERT INTO public_groups (id, name, position) VALUES (?, ?, ?);

-- name: InsertPublicEntry :exec
INSERT INTO public_entries (subject_type, subject_id, group_id, position) VALUES (?, ?, ?, ?);

-- name: ListLayoutDNSRecords :many
SELECT r.id, r.name, r.display_name, r.dashboard, r.monitoring, z.name AS zone_name
FROM dns_records r JOIN dns_zones z ON z.id = r.zone_id WHERE r.dashboard = 1 OR r.monitoring = 1;

-- name: SetMonitorEndpointGroup :exec
UPDATE monitor_endpoints SET group_name = ?, updated_at = ? WHERE id = ?;

-- name: SetMonitorEndpointPublic :exec
UPDATE monitor_endpoints SET public = ?, updated_at = ? WHERE id = ?;

-- name: SetDNSRecordDashboard :exec
UPDATE dns_records SET dashboard = ?, updated_at = ? WHERE id = ?;

-- Public history (07 section 7.1): rounds of the last hour with their response time and hourly round rollups of the last
-- 90 days, for every endpoint (endpoint NULL) or one. Naming the endpoints lets SQLite seek through the primary key; by
-- time alone it read every check's rows (about 150 ms on a copy of prod, 2026-11-22).
-- name: ListRoundSamplesSince :many
SELECT endpoint_id, at, verdict, latency_ms FROM monitor_samples
WHERE endpoint_id IN (SELECT id FROM monitor_endpoints WHERE id = sqlc.narg('endpoint') OR sqlc.narg('endpoint') IS NULL)
  AND check_id = '' AND at >= sqlc.arg('since')
ORDER BY endpoint_id, at;

-- name: ListHourlyRoundRollupsSince :many
SELECT endpoint_id, bucket, count, ok_count, avg_ms, p95_ms FROM monitor_rollups
WHERE endpoint_id IN (SELECT id FROM monitor_endpoints WHERE id = sqlc.narg('endpoint') OR sqlc.narg('endpoint') IS NULL)
  AND check_id = '' AND resolution = 3600 AND bucket >= sqlc.arg('since');

-- State changes of the last 90 days (small: one row per change).
-- name: ListMonitorEventsSince :many
SELECT endpoint_id, at, from_state, to_state FROM monitor_events WHERE at >= ? ORDER BY at;

-- name: LatestCheckSampleDetail :one
SELECT detail FROM monitor_samples WHERE endpoint_id = ? AND check_id = ? ORDER BY at DESC LIMIT 1;
