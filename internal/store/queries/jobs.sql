-- name: UpsertJob :exec
-- Registers a job kind; an existing row keeps its enabled flag and next run.
INSERT INTO jobs (kind, interval_seconds, enabled, next_run_at, created_at)
VALUES (?, ?, 1, ?, ?)
ON CONFLICT (kind) DO UPDATE SET interval_seconds = excluded.interval_seconds;

-- name: DueJobs :many
SELECT kind, interval_seconds, enabled, next_run_at, created_at
FROM jobs
WHERE enabled = 1 AND next_run_at <= ?
ORDER BY next_run_at;

-- name: SetJobNextRun :exec
UPDATE jobs SET next_run_at = ? WHERE kind = ?;

-- name: StartJobRun :exec
INSERT INTO job_runs (id, kind, started_at, status) VALUES (?, ?, ?, 'running');

-- name: FinishJobRun :exec
UPDATE job_runs
SET finished_at = ?, status = ?, duration_ms = ?, summary = ?, error = ?
WHERE id = ?;

-- name: InterruptRunningJobRuns :execrows
-- Runs left 'running' by a previous process (crash, restart) can never finish.
UPDATE job_runs SET status = 'interrupted', finished_at = ?, error = 'process stopped before the run finished'
WHERE status = 'running';

-- name: ListJobsWithLastRun :many
SELECT j.kind, j.interval_seconds, j.enabled, j.next_run_at,
       r.id AS run_id, r.started_at, r.finished_at, r.status, r.duration_ms, r.summary, r.error
FROM jobs j
LEFT JOIN job_runs r ON r.id = (
    SELECT id FROM job_runs WHERE kind = j.kind ORDER BY started_at DESC, id DESC LIMIT 1
)
ORDER BY j.kind;
