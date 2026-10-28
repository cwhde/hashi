-- name: InsertSyncPlan :exec
INSERT INTO sync_plans (id, subsystem, target, title, trigger, risk, status, plan_hash, created_by, created_at, approved_by, approved_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: InsertSyncPlanItem :exec
INSERT INTO sync_plan_items (plan_id, seq, action, object, before_json, after_json, risk, note) VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetSyncPlan :one
SELECT * FROM sync_plans WHERE id = ?;

-- name: GetOpenSyncPlan :one
SELECT * FROM sync_plans WHERE subsystem = ? AND target = ? AND status IN ('pending', 'approved', 'applying');

-- name: ListSyncPlanItems :many
SELECT * FROM sync_plan_items WHERE plan_id = ? ORDER BY seq;

-- name: ListOpenSyncPlans :many
SELECT * FROM sync_plans WHERE status IN ('pending', 'approved', 'applying') ORDER BY created_at DESC, id DESC;

-- name: ListRecentSyncPlans :many
SELECT * FROM sync_plans WHERE status NOT IN ('pending', 'approved', 'applying')
ORDER BY created_at DESC, id DESC LIMIT ?;

-- name: SetSyncPlanStatus :execrows
-- Moves a plan only from the status the caller saw (no lost updates between two requests).
UPDATE sync_plans SET status = sqlc.arg(status) WHERE id = sqlc.arg(id) AND status = sqlc.arg(from_status);

-- name: ApproveSyncPlan :execrows
UPDATE sync_plans SET status = 'approved', approved_by = ?, approved_at = ? WHERE id = ? AND status = 'pending';

-- name: FinishSyncPlan :exec
UPDATE sync_plans SET status = ?, finished_at = ?, result = ?, error = ? WHERE id = ?;

-- name: InsertSyncRun :exec
INSERT INTO sync_runs (id, plan_id, started_at) VALUES (?, ?, ?);

-- name: FinishSyncRun :exec
UPDATE sync_runs SET finished_at = ?, outcome = ?, summary = ?, error = ? WHERE id = ?;

-- name: ListSyncRuns :many
SELECT r.*, p.subsystem, p.title FROM sync_runs r JOIN sync_plans p ON p.id = r.plan_id
ORDER BY r.started_at DESC, r.id DESC LIMIT ?;

-- name: AcquireSyncLock :execrows
INSERT INTO sync_locks (key, holder, acquired_at) VALUES (?, ?, ?) ON CONFLICT (key) DO NOTHING;

-- name: ReleaseSyncLock :exec
DELETE FROM sync_locks WHERE key = ? AND holder = ?;

-- name: ClearSyncLocks :exec
DELETE FROM sync_locks;

-- name: ResetInterruptedSyncPlans :execrows
-- At start: a plan left in 'applying' by a crash failed; the next plan re-reads the provider.
UPDATE sync_plans SET status = 'failed', finished_at = ?, error = 'Hashi stopped while applying; the next sync re-reads the provider'
WHERE status = 'applying';
