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

-- sync_plans.subsystem gains scripts (rebuild as in 00014).
CREATE TABLE sync_plans_v24 (
    id          TEXT PRIMARY KEY,
    subsystem   TEXT NOT NULL CHECK (subsystem IN ('dns-records', 'dns-record-deletes', 'dns-prune', 'dns-probe',
                                                   'traefik', 'certificates', 'firewall', 'adguard', 'scripts')),
    target      TEXT NOT NULL,
    title       TEXT NOT NULL,
    trigger     TEXT NOT NULL CHECK (trigger IN ('save', 'reconcile', 'manual', 'probe', 'prune')),
    risk        TEXT NOT NULL CHECK (risk IN ('low', 'high')),
    status      TEXT NOT NULL CHECK (status IN ('pending', 'approved', 'applying', 'applied', 'failed', 'discarded', 'superseded')),
    plan_hash   TEXT NOT NULL,
    created_by  TEXT NOT NULL,
    created_at  INTEGER NOT NULL,
    approved_by TEXT NOT NULL DEFAULT '',
    approved_at INTEGER,
    finished_at INTEGER,
    result      TEXT NOT NULL DEFAULT '',
    error       TEXT NOT NULL DEFAULT ''
) STRICT;
INSERT INTO sync_plans_v24 SELECT * FROM sync_plans;

CREATE TABLE sync_plan_items_v24 (
    plan_id     TEXT NOT NULL REFERENCES sync_plans_v24 (id) ON DELETE CASCADE,
    seq         INTEGER NOT NULL,
    action      TEXT NOT NULL CHECK (action IN ('create', 'update', 'delete', 'label', 'conflict')),
    object      TEXT NOT NULL,
    before_json TEXT NOT NULL DEFAULT '',
    after_json  TEXT NOT NULL DEFAULT '',
    risk        TEXT NOT NULL CHECK (risk IN ('low', 'high')),
    note        TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (plan_id, seq)
) STRICT;
INSERT INTO sync_plan_items_v24 SELECT * FROM sync_plan_items;

CREATE TABLE sync_runs_v24 (
    id          TEXT PRIMARY KEY,
    plan_id     TEXT NOT NULL REFERENCES sync_plans_v24 (id) ON DELETE CASCADE,
    started_at  INTEGER NOT NULL,
    finished_at INTEGER,
    outcome     TEXT NOT NULL DEFAULT 'running'
                CHECK (outcome IN ('running', 'applied', 'failed', 'replanned', 'no_changes')),
    summary     TEXT NOT NULL DEFAULT '',
    error       TEXT NOT NULL DEFAULT ''
) STRICT;
INSERT INTO sync_runs_v24 SELECT * FROM sync_runs;

DROP TABLE sync_plan_items;
DROP TABLE sync_runs;
DROP TABLE sync_plans;
-- Renaming the parent rewrites the children's REFERENCES to the new name.
ALTER TABLE sync_plans_v24 RENAME TO sync_plans;
ALTER TABLE sync_plan_items_v24 RENAME TO sync_plan_items;
ALTER TABLE sync_runs_v24 RENAME TO sync_runs;
CREATE UNIQUE INDEX sync_plans_open ON sync_plans (subsystem, target) WHERE status IN ('pending', 'approved', 'applying');
CREATE INDEX sync_plans_created ON sync_plans (created_at);
CREATE INDEX sync_runs_started ON sync_runs (started_at);

-- +goose Down
DELETE FROM sync_plans WHERE subsystem = 'scripts';
CREATE TABLE sync_plans_v24d (
    id          TEXT PRIMARY KEY,
    subsystem   TEXT NOT NULL CHECK (subsystem IN ('dns-records', 'dns-record-deletes', 'dns-prune', 'dns-probe',
                                                   'traefik', 'certificates', 'firewall', 'adguard')),
    target      TEXT NOT NULL,
    title       TEXT NOT NULL,
    trigger     TEXT NOT NULL CHECK (trigger IN ('save', 'reconcile', 'manual', 'probe', 'prune')),
    risk        TEXT NOT NULL CHECK (risk IN ('low', 'high')),
    status      TEXT NOT NULL CHECK (status IN ('pending', 'approved', 'applying', 'applied', 'failed', 'discarded', 'superseded')),
    plan_hash   TEXT NOT NULL,
    created_by  TEXT NOT NULL,
    created_at  INTEGER NOT NULL,
    approved_by TEXT NOT NULL DEFAULT '',
    approved_at INTEGER,
    finished_at INTEGER,
    result      TEXT NOT NULL DEFAULT '',
    error       TEXT NOT NULL DEFAULT ''
) STRICT;
INSERT INTO sync_plans_v24d SELECT * FROM sync_plans;

CREATE TABLE sync_plan_items_v24d (
    plan_id     TEXT NOT NULL REFERENCES sync_plans_v24d (id) ON DELETE CASCADE,
    seq         INTEGER NOT NULL,
    action      TEXT NOT NULL CHECK (action IN ('create', 'update', 'delete', 'label', 'conflict')),
    object      TEXT NOT NULL,
    before_json TEXT NOT NULL DEFAULT '',
    after_json  TEXT NOT NULL DEFAULT '',
    risk        TEXT NOT NULL CHECK (risk IN ('low', 'high')),
    note        TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (plan_id, seq)
) STRICT;
INSERT INTO sync_plan_items_v24d SELECT * FROM sync_plan_items;

CREATE TABLE sync_runs_v24d (
    id          TEXT PRIMARY KEY,
    plan_id     TEXT NOT NULL REFERENCES sync_plans_v24d (id) ON DELETE CASCADE,
    started_at  INTEGER NOT NULL,
    finished_at INTEGER,
    outcome     TEXT NOT NULL DEFAULT 'running'
                CHECK (outcome IN ('running', 'applied', 'failed', 'replanned', 'no_changes')),
    summary     TEXT NOT NULL DEFAULT '',
    error       TEXT NOT NULL DEFAULT ''
) STRICT;
INSERT INTO sync_runs_v24d SELECT * FROM sync_runs;

DROP TABLE sync_plan_items;
DROP TABLE sync_runs;
DROP TABLE sync_plans;
-- Renaming the parent rewrites the children's REFERENCES to the new name.
ALTER TABLE sync_plans_v24d RENAME TO sync_plans;
ALTER TABLE sync_plan_items_v24d RENAME TO sync_plan_items;
ALTER TABLE sync_runs_v24d RENAME TO sync_runs;
CREATE UNIQUE INDEX sync_plans_open ON sync_plans (subsystem, target) WHERE status IN ('pending', 'approved', 'applying');
CREATE INDEX sync_plans_created ON sync_plans (created_at);
CREATE INDEX sync_runs_started ON sync_runs (started_at);
DROP TABLE script_runs;
DROP TABLE script_env;
DROP TABLE script_targets;
DROP TABLE scripts;
