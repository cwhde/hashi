-- name: ListAdGuardConnections :many
SELECT * FROM adguard_connections ORDER BY name;

-- name: GetAdGuardConnection :one
SELECT * FROM adguard_connections WHERE id = ?;

-- name: ListAdGuardConnectionsBySet :many
SELECT * FROM adguard_connections WHERE target_set_id = ? ORDER BY name;

-- name: InsertAdGuardConnection :exec
INSERT INTO adguard_connections (id, name, target_set_id, port, scheme, path_prefix, tls_verify, server_name, username, secret_id, mode, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateAdGuardConnection :exec
UPDATE adguard_connections SET name = ?, target_set_id = ?, port = ?, scheme = ?, path_prefix = ?, tls_verify = ?, server_name = ?,
    username = ?, secret_id = ?, mode = ?, updated_at = ? WHERE id = ?;

-- name: SetAdGuardConnectionCheck :exec
UPDATE adguard_connections SET last_checked_at = ?, last_version = ?, last_error = ? WHERE id = ?;

-- name: DeleteAdGuardConnection :exec
DELETE FROM adguard_connections WHERE id = ?;

-- name: ListAdGuardOwned :many
SELECT * FROM adguard_rewrites_owned WHERE connection_id = ? ORDER BY domain, answer;

-- name: PutAdGuardOwned :exec
INSERT INTO adguard_rewrites_owned (connection_id, domain, answer, origin, hash, created_at) VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (connection_id, domain, answer) DO UPDATE SET origin = excluded.origin, hash = excluded.hash;

-- name: DeleteAdGuardOwned :exec
DELETE FROM adguard_rewrites_owned WHERE connection_id = ? AND domain = ? AND answer = ?;
