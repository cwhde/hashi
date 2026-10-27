-- +goose Up
-- Timestamps are integer Unix milliseconds UTC (invariant 6), IDs are ULID text.

CREATE TABLE app_settings (
    key        TEXT PRIMARY KEY,
    value_json TEXT NOT NULL,
    updated_at INTEGER NOT NULL
) STRICT;

CREATE TABLE jobs (
    kind             TEXT PRIMARY KEY,
    interval_seconds INTEGER NOT NULL CHECK (interval_seconds > 0),
    enabled          INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    next_run_at      INTEGER NOT NULL,
    created_at       INTEGER NOT NULL
) STRICT;

CREATE TABLE job_runs (
    id          TEXT PRIMARY KEY,
    kind        TEXT NOT NULL REFERENCES jobs (kind) ON DELETE CASCADE,
    started_at  INTEGER NOT NULL,
    finished_at INTEGER,
    status      TEXT NOT NULL CHECK (status IN ('running', 'succeeded', 'failed', 'interrupted')),
    duration_ms INTEGER,
    summary     TEXT NOT NULL DEFAULT '',
    error       TEXT NOT NULL DEFAULT ''
) STRICT;

CREATE INDEX job_runs_kind_started ON job_runs (kind, started_at DESC);

-- +goose Down
DROP TABLE job_runs;
DROP TABLE jobs;
DROP TABLE app_settings;
