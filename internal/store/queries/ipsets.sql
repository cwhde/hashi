-- name: ListIPSets :many
SELECT * FROM ip_sets ORDER BY kind = 'system' DESC, name, id;

-- name: GetIPSet :one
SELECT * FROM ip_sets WHERE id = ?;

-- name: GetIPSetBySystemKey :one
SELECT * FROM ip_sets WHERE system_key = ?;

-- name: InsertIPSet :exec
INSERT INTO ip_sets (id, slug, name, description, kind, origin, system_key, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateIPSet :exec
UPDATE ip_sets SET slug = ?, name = ?, description = ?, updated_at = ? WHERE id = ?;

-- name: DeleteIPSet :exec
DELETE FROM ip_sets WHERE id = ?;

-- name: SetIPSetCompiled :exec
UPDATE ip_sets SET compiled_hash = ?, resolution_status = ?, resolution_problems_json = ?, compiled_at = ? WHERE id = ?;

-- name: ListAllIPSetElements :many
SELECT * FROM ip_set_elements ORDER BY ip_set_id, position;

-- name: ListIPSetElements :many
SELECT * FROM ip_set_elements WHERE ip_set_id = ? ORDER BY position;

-- name: GetIPSetElement :one
SELECT * FROM ip_set_elements WHERE id = ?;

-- name: NextIPSetElementPosition :one
SELECT CAST(COALESCE(MAX(position), 0) + 1 AS INTEGER) AS next FROM ip_set_elements WHERE ip_set_id = ?;

-- name: InsertIPSetElement :exec
INSERT INTO ip_set_elements (id, ip_set_id, position, type, origin, locked, address, prefix, family, host_id, scope,
    fqdn, child_set_id, note, expires_at, created_by, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateIPSetElement :exec
UPDATE ip_set_elements
SET type = ?, address = ?, prefix = ?, family = ?, host_id = ?, scope = ?, fqdn = ?, child_set_id = ?, note = ?,
    expires_at = ?, updated_at = ?
WHERE id = ?;

-- name: DeleteIPSetElement :exec
DELETE FROM ip_set_elements WHERE id = ?;

-- name: ListIPSetParents :many
SELECT DISTINCT s.* FROM ip_sets s JOIN ip_set_elements e ON e.ip_set_id = s.id WHERE e.child_set_id = ? ORDER BY s.name;
