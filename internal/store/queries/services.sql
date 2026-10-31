-- name: InsertService :exec
INSERT INTO services (id, name, slug, kind, domain_mode, host_name, enabled, origin, owner_feature, target_address,
    target_port, tls_skip_verify, tls_server_name, tls_min_version, advanced_routes, dashboard_visible,
    monitoring_visible, adguard_rewrite, target_set_id, entry_host_id, public_port, proxy_protocol, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateService :exec
UPDATE services
SET name = ?, slug = ?, kind = ?, domain_mode = ?, host_name = ?, target_address = ?, target_port = ?,
    tls_skip_verify = ?, tls_server_name = ?, tls_min_version = ?, advanced_routes = ?, dashboard_visible = ?,
    monitoring_visible = ?, adguard_rewrite = ?, target_set_id = ?, entry_host_id = ?, public_port = ?, proxy_protocol = ?,
    updated_at = ?
WHERE id = ?;

-- name: SetServiceEnabled :exec
UPDATE services SET enabled = ?, updated_at = ? WHERE id = ?;

-- name: GetService :one
SELECT * FROM services WHERE id = ?;

-- name: GetServiceByOwner :one
SELECT * FROM services WHERE owner_feature = ?;

-- name: ListServices :many
SELECT * FROM services ORDER BY origin = 'system' DESC, name, id;

-- name: DeleteService :exec
DELETE FROM services WHERE id = ?;

-- name: InsertServiceRoute :exec
INSERT INTO service_routes (id, service_id, enabled, targets_json, path_match, path_value, priority, rewrite_mode,
    rewrite_pattern, rewrite_value, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: DeleteServiceRoutes :exec
DELETE FROM service_routes WHERE service_id = ?;

-- name: ListServiceRoutes :many
SELECT * FROM service_routes WHERE service_id = ? ORDER BY priority DESC, id;

-- name: ListAllServiceRoutes :many
SELECT * FROM service_routes ORDER BY service_id, priority DESC, id;

-- name: ListServicesByEntryHost :many
SELECT id, name FROM services WHERE entry_host_id = ? ORDER BY name;
