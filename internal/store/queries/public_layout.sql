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
