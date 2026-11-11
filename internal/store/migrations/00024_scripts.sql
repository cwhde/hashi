-- +goose Up
-- Scripts (09 §1): shell scripts Hashi keeps on firewall hosts and runs by hand or on a schedule.
CREATE TABLE scripts (
    id              TEXT PRIMARY KEY,
    slug            TEXT NOT NULL UNIQUE CHECK (slug GLOB '[a-z0-9]*' AND slug NOT GLOB '*[^a-z0-9-]*' AND length(slug) <= 48),
    name            TEXT NOT NULL UNIQUE,
    description     TEXT NOT NULL DEFAULT '',
    body            TEXT NOT NULL,
    enabled         INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    -- Five-field cron; '' = manual only.
    schedule        TEXT NOT NULL DEFAULT '',
    timeout_seconds INTEGER NOT NULL DEFAULT 600 CHECK (timeout_seconds BETWEEN 1 AND 86400),
    -- 1: every firewall host; 0: only the hosts in script_targets (none listed = no host, never all).
    all_hosts       INTEGER NOT NULL DEFAULT 1 CHECK (all_hosts IN (0, 1)),
    created_at      INTEGER NOT NULL,
    updated_at      INTEGER NOT NULL
) STRICT;

CREATE TABLE script_targets (
    script_id TEXT NOT NULL REFERENCES scripts (id) ON DELETE CASCADE,
    host_id   TEXT NOT NULL REFERENCES firewall_hosts (id) ON DELETE CASCADE,
    PRIMARY KEY (script_id, host_id)
) STRICT;

-- Environment variables. A secret one keeps its value in the vault (secret_id) and '' here.
CREATE TABLE script_env (
    script_id TEXT NOT NULL REFERENCES scripts (id) ON DELETE CASCADE,
    name      TEXT NOT NULL CHECK (name GLOB '[A-Za-z_]*' AND name NOT GLOB '*[^A-Za-z0-9_]*'),
    value     TEXT NOT NULL DEFAULT '',
    secret_id TEXT REFERENCES secret_records (id),
    PRIMARY KEY (script_id, name)
) STRICT;

-- One run on one host. Output is capped and redacted before it is stored. The newest 100 per script are kept.
CREATE TABLE script_runs (
    id          TEXT PRIMARY KEY,
    script_id   TEXT NOT NULL REFERENCES scripts (id) ON DELETE CASCADE,
    host_id     TEXT NOT NULL REFERENCES firewall_hosts (id) ON DELETE CASCADE,
    trigger     TEXT NOT NULL CHECK (trigger IN ('manual', 'scheduled')),
    status      TEXT NOT NULL CHECK (status IN ('running', 'succeeded', 'failed', 'timed_out', 'error')),
    exit_code   INTEGER,
    started_at  INTEGER NOT NULL,
    finished_at INTEGER,
    output      TEXT NOT NULL DEFAULT '',
    truncated   INTEGER NOT NULL DEFAULT 0 CHECK (truncated IN (0, 1)),
    error       TEXT NOT NULL DEFAULT '',
    actor       TEXT NOT NULL DEFAULT '',
    -- The spool file a scheduled run came from, so a collection that dies half way never stores it twice.
    spool_name  TEXT NOT NULL DEFAULT ''
) STRICT;
CREATE INDEX script_runs_script ON script_runs (script_id, started_at DESC);
CREATE UNIQUE INDEX script_runs_spool ON script_runs (host_id, spool_name) WHERE spool_name != '';

-- +goose Down
DROP TABLE script_runs;
DROP TABLE script_env;
DROP TABLE script_targets;
DROP TABLE scripts;
