-- name: InsertScript :exec
INSERT INTO scripts (id, slug, name, description, body, enabled, schedule, timeout_seconds, all_hosts, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateScript :exec
UPDATE scripts SET name = ?, description = ?, body = ?, enabled = ?, schedule = ?, timeout_seconds = ?, all_hosts = ?,
    updated_at = ? WHERE id = ?;

-- name: GetScript :one
SELECT * FROM scripts WHERE id = ?;

-- name: ListScripts :many
SELECT * FROM scripts ORDER BY name;

-- name: DeleteScript :exec
DELETE FROM scripts WHERE id = ?;

-- name: ListScriptTargets :many
SELECT host_id FROM script_targets WHERE script_id = ? ORDER BY host_id;

-- name: ListAllScriptTargets :many
SELECT * FROM script_targets ORDER BY script_id, host_id;

-- name: DeleteScriptTargets :exec
DELETE FROM script_targets WHERE script_id = ?;

-- name: InsertScriptTarget :exec
INSERT INTO script_targets (script_id, host_id) VALUES (?, ?);

-- name: ListScriptEnv :many
SELECT * FROM script_env WHERE script_id = ? ORDER BY name;

-- name: DeleteScriptEnv :exec
DELETE FROM script_env WHERE script_id = ?;

-- name: InsertScriptEnv :exec
INSERT INTO script_env (script_id, name, value, secret_id) VALUES (?, ?, ?, ?);

-- name: InsertScriptRun :exec
INSERT INTO script_runs (id, script_id, host_id, trigger, status, exit_code, started_at, finished_at, output, truncated,
    error, actor, spool_name) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: FinishScriptRun :exec
UPDATE script_runs SET status = ?, exit_code = ?, finished_at = ?, output = ?, truncated = ?, error = ? WHERE id = ?;

-- name: GetScriptRun :one
SELECT * FROM script_runs WHERE id = ?;

-- name: ListScriptRuns :many
SELECT * FROM script_runs WHERE script_id = ? ORDER BY started_at DESC, id DESC LIMIT 100;

-- name: CountScriptSpool :one
SELECT COUNT(*) FROM script_runs WHERE host_id = ? AND spool_name = ?;

-- name: PruneScriptRuns :exec
DELETE FROM script_runs WHERE script_runs.script_id = sqlc.arg(script_id) AND script_runs.id NOT IN (
    SELECT r.id FROM script_runs AS r WHERE r.script_id = sqlc.arg(script_id) ORDER BY r.started_at DESC, r.id DESC LIMIT 100);

-- name: InterruptScriptRuns :exec
UPDATE script_runs SET status = 'error', error = 'Hashi restarted during the run', finished_at = ? WHERE status = 'running';

-- name: ListScriptRunStatuses :many
SELECT status FROM script_runs WHERE script_id = ? ORDER BY started_at DESC, id DESC LIMIT 10;

-- name: ListScriptRunsBrief :many
SELECT id, script_id, host_id, trigger, status, exit_code, started_at, finished_at, truncated, error, actor
FROM script_runs WHERE script_id = ? ORDER BY started_at DESC, id DESC LIMIT 100;
