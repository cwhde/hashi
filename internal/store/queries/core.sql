-- name: InsertAuditEvent :exec
INSERT INTO audit_events (id, at, actor, action, target, outcome, ip, detail_json) VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- Newest first; cursor is the last seen id (ULIDs sort by time).
-- name: ListAuditEvents :many
SELECT * FROM audit_events
WHERE (CAST(sqlc.arg(cursor) AS TEXT) = '' OR id < CAST(sqlc.arg(cursor) AS TEXT))
  AND (CAST(sqlc.arg(target) AS TEXT) = '' OR target = CAST(sqlc.arg(target) AS TEXT))
ORDER BY id DESC
LIMIT sqlc.arg(lim);

-- name: ListSetupSteps :many
SELECT * FROM setup_state ORDER BY step;

-- name: UpsertSetupStep :exec
INSERT INTO setup_state (step, status, inputs_json, last_error, updated_at) VALUES (?, ?, ?, ?, ?)
ON CONFLICT (step) DO UPDATE SET status = excluded.status, inputs_json = excluded.inputs_json,
    last_error = excluded.last_error, updated_at = excluded.updated_at;

-- name: GetSetting :one
SELECT * FROM app_settings WHERE key = ?;

-- name: ListSettings :many
SELECT * FROM app_settings ORDER BY key;

-- name: UpsertSetting :exec
INSERT INTO app_settings (key, value_json, updated_at) VALUES (?, ?, ?)
ON CONFLICT (key) DO UPDATE SET value_json = excluded.value_json, updated_at = excluded.updated_at;
