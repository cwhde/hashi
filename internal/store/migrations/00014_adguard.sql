-- +goose Up
-- S8: AdGuard Home connections and the rewrites Hashi owns on them (03 §8). AdGuard has no comments, so ownership is
-- kept here. The password is in the vault (invariant 5). A new connection starts read-only: it writes after the admin says so.
CREATE TABLE adguard_connections (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL UNIQUE,
    target_set_id   TEXT NOT NULL REFERENCES ip_sets (id) ON DELETE RESTRICT,
    port            INTEGER NOT NULL CHECK (port BETWEEN 1 AND 65535),
    scheme          TEXT NOT NULL CHECK (scheme IN ('http', 'https')),
    path_prefix     TEXT NOT NULL DEFAULT '',
    tls_verify      INTEGER NOT NULL DEFAULT 1 CHECK (tls_verify IN (0, 1)),
    server_name     TEXT NOT NULL DEFAULT '',
    username        TEXT NOT NULL DEFAULT '',
    secret_id       TEXT REFERENCES secret_records (id) ON DELETE SET NULL,
    mode            TEXT NOT NULL DEFAULT 'read_only' CHECK (mode IN ('active', 'read_only', 'disabled')),
    last_checked_at INTEGER,
    last_version    TEXT NOT NULL DEFAULT '',
    last_error      TEXT NOT NULL DEFAULT '',
    created_at      INTEGER NOT NULL,
    updated_at      INTEGER NOT NULL
) STRICT;

CREATE TABLE adguard_rewrites_owned (
    connection_id TEXT NOT NULL REFERENCES adguard_connections (id) ON DELETE CASCADE,
    domain        TEXT NOT NULL,
    answer        TEXT NOT NULL,
    origin        TEXT NOT NULL,
    hash          TEXT NOT NULL,
    created_at    INTEGER NOT NULL,
    PRIMARY KEY (connection_id, domain, answer)
) STRICT;

-- sync_plans.subsystem gains adguard (rebuild as in 00008).
CREATE TABLE sync_plans_v14 (
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
INSERT INTO sync_plans_v14 SELECT * FROM sync_plans;

CREATE TABLE sync_plan_items_v14 (
    plan_id     TEXT NOT NULL REFERENCES sync_plans_v14 (id) ON DELETE CASCADE,
    seq         INTEGER NOT NULL,
    action      TEXT NOT NULL CHECK (action IN ('create', 'update', 'delete', 'label', 'conflict')),
    object      TEXT NOT NULL,
    before_json TEXT NOT NULL DEFAULT '',
    after_json  TEXT NOT NULL DEFAULT '',
    risk        TEXT NOT NULL CHECK (risk IN ('low', 'high')),
    note        TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (plan_id, seq)
) STRICT;
INSERT INTO sync_plan_items_v14 SELECT * FROM sync_plan_items;

CREATE TABLE sync_runs_v14 (
    id          TEXT PRIMARY KEY,
    plan_id     TEXT NOT NULL REFERENCES sync_plans_v14 (id) ON DELETE CASCADE,
    started_at  INTEGER NOT NULL,
    finished_at INTEGER,
    outcome     TEXT NOT NULL DEFAULT 'running'
                CHECK (outcome IN ('running', 'applied', 'failed', 'replanned', 'no_changes')),
    summary     TEXT NOT NULL DEFAULT '',
    error       TEXT NOT NULL DEFAULT ''
) STRICT;
INSERT INTO sync_runs_v14 SELECT * FROM sync_runs;

DROP TABLE sync_plan_items;
DROP TABLE sync_runs;
DROP TABLE sync_plans;
-- Renaming the parent rewrites the children's REFERENCES to the new name.
ALTER TABLE sync_plans_v14 RENAME TO sync_plans;
ALTER TABLE sync_plan_items_v14 RENAME TO sync_plan_items;
ALTER TABLE sync_runs_v14 RENAME TO sync_runs;
CREATE UNIQUE INDEX sync_plans_open ON sync_plans (subsystem, target) WHERE status IN ('pending', 'approved', 'applying');
CREATE INDEX sync_plans_created ON sync_plans (created_at);
CREATE INDEX sync_runs_started ON sync_runs (started_at);

-- +goose Down
DELETE FROM sync_plans WHERE subsystem = 'adguard';
CREATE TABLE sync_plans_v14d (
    id          TEXT PRIMARY KEY,
    subsystem   TEXT NOT NULL CHECK (subsystem IN ('dns-records', 'dns-record-deletes', 'dns-prune', 'dns-probe',
                                                   'traefik', 'certificates', 'firewall')),
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
INSERT INTO sync_plans_v14d SELECT * FROM sync_plans;

CREATE TABLE sync_plan_items_v14d (
    plan_id     TEXT NOT NULL REFERENCES sync_plans_v14d (id) ON DELETE CASCADE,
    seq         INTEGER NOT NULL,
    action      TEXT NOT NULL CHECK (action IN ('create', 'update', 'delete', 'label', 'conflict')),
    object      TEXT NOT NULL,
    before_json TEXT NOT NULL DEFAULT '',
    after_json  TEXT NOT NULL DEFAULT '',
    risk        TEXT NOT NULL CHECK (risk IN ('low', 'high')),
    note        TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (plan_id, seq)
) STRICT;
INSERT INTO sync_plan_items_v14d SELECT * FROM sync_plan_items;

CREATE TABLE sync_runs_v14d (
    id          TEXT PRIMARY KEY,
    plan_id     TEXT NOT NULL REFERENCES sync_plans_v14d (id) ON DELETE CASCADE,
    started_at  INTEGER NOT NULL,
    finished_at INTEGER,
    outcome     TEXT NOT NULL DEFAULT 'running'
                CHECK (outcome IN ('running', 'applied', 'failed', 'replanned', 'no_changes')),
    summary     TEXT NOT NULL DEFAULT '',
    error       TEXT NOT NULL DEFAULT ''
) STRICT;
INSERT INTO sync_runs_v14d SELECT * FROM sync_runs;

DROP TABLE sync_plan_items;
DROP TABLE sync_runs;
DROP TABLE sync_plans;
-- Renaming the parent rewrites the children's REFERENCES to the new name.
ALTER TABLE sync_plans_v14d RENAME TO sync_plans;
ALTER TABLE sync_plan_items_v14d RENAME TO sync_plan_items;
ALTER TABLE sync_runs_v14d RENAME TO sync_runs;
CREATE UNIQUE INDEX sync_plans_open ON sync_plans (subsystem, target) WHERE status IN ('pending', 'approved', 'applying');
CREATE INDEX sync_plans_created ON sync_plans (created_at);
CREATE INDEX sync_runs_started ON sync_runs (started_at);
DROP TABLE adguard_rewrites_owned;
DROP TABLE adguard_connections;
