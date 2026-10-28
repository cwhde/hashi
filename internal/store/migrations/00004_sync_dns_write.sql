-- +goose Up
-- S3: the sync engine's plans, items, runs and locks (01-platform.md §5) and the desired DNS records (03-dns.md §6–7).

-- Desired RRSets Hashi owns: manual entries (origin user) and imported ones (origin imported, copied from the observed
-- snapshot when the import decision is saved). Ownership stays authoritative in dns_ownership (invariant 1).
-- name is relative to the zone ('@' = apex); ttl NULL = zone default; values are the decoded form (TXT unquoted).
CREATE TABLE dns_records (
    id           TEXT PRIMARY KEY,
    zone_id      TEXT NOT NULL REFERENCES dns_zones (id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    type         TEXT NOT NULL CHECK (type NOT IN ('NS', 'SOA')), -- invariant 2
    ttl          INTEGER CHECK (ttl IS NULL OR ttl BETWEEN 60 AND 86400),
    values_json  TEXT NOT NULL,
    origin       TEXT NOT NULL CHECK (origin IN ('system', 'managed', 'imported', 'user')),
    -- Manual entry fields for the dashboard and monitoring (03 §7); consumers arrive with S7.
    display_name TEXT NOT NULL DEFAULT '',
    dashboard    INTEGER NOT NULL DEFAULT 0 CHECK (dashboard IN (0, 1)),
    monitoring   INTEGER NOT NULL DEFAULT 0 CHECK (monitoring IN (0, 1)),
    created_at   INTEGER NOT NULL,
    updated_at   INTEGER NOT NULL,
    UNIQUE (zone_id, name, type)
) STRICT;

-- A persisted diff between desired and observed state for one subsystem and target (01 §5.1).
-- subsystem: dns-records (desired vs observed), dns-prune (prune decisions), dns-probe (write test, invariant 4).
-- At most one open plan per subsystem and target: a newer plan supersedes the older one.
CREATE TABLE sync_plans (
    id          TEXT PRIMARY KEY,
    subsystem   TEXT NOT NULL CHECK (subsystem IN ('dns-records', 'dns-prune', 'dns-probe')),
    target      TEXT NOT NULL,
    title       TEXT NOT NULL,
    trigger     TEXT NOT NULL CHECK (trigger IN ('save', 'reconcile', 'manual', 'probe', 'prune')),
    risk        TEXT NOT NULL CHECK (risk IN ('low', 'high')),
    status      TEXT NOT NULL CHECK (status IN ('pending', 'approved', 'applying', 'applied', 'failed', 'discarded', 'superseded')),
    -- sha256 over the items: what is approved is exactly what is applied (01 §5.1 step 2).
    plan_hash   TEXT NOT NULL,
    created_by  TEXT NOT NULL,
    created_at  INTEGER NOT NULL,
    approved_by TEXT NOT NULL DEFAULT '',
    approved_at INTEGER,
    finished_at INTEGER,
    result      TEXT NOT NULL DEFAULT '',
    error       TEXT NOT NULL DEFAULT ''
) STRICT;

CREATE UNIQUE INDEX sync_plans_open ON sync_plans (subsystem, target) WHERE status IN ('pending', 'approved', 'applying');
CREATE INDEX sync_plans_created ON sync_plans (created_at);

-- One change of a plan, with exact before/after values (01 §5.1, 03 §6). before/after are JSON objects of the
-- subsystem (DNS: {"name","type","ttl","values"}); '' when absent.
CREATE TABLE sync_plan_items (
    plan_id     TEXT NOT NULL REFERENCES sync_plans (id) ON DELETE CASCADE,
    seq         INTEGER NOT NULL,
    action      TEXT NOT NULL CHECK (action IN ('create', 'update', 'delete', 'label', 'conflict')),
    object      TEXT NOT NULL,
    before_json TEXT NOT NULL DEFAULT '',
    after_json  TEXT NOT NULL DEFAULT '',
    risk        TEXT NOT NULL CHECK (risk IN ('low', 'high')),
    note        TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (plan_id, seq)
) STRICT;

-- Every apply attempt with its outcome (invariant 15: last run, duration, diff summary, error).
CREATE TABLE sync_runs (
    id          TEXT PRIMARY KEY,
    plan_id     TEXT NOT NULL REFERENCES sync_plans (id) ON DELETE CASCADE,
    started_at  INTEGER NOT NULL,
    finished_at INTEGER,
    outcome     TEXT NOT NULL DEFAULT 'running'
                CHECK (outcome IN ('running', 'applied', 'failed', 'replanned', 'no_changes')),
    summary     TEXT NOT NULL DEFAULT '',
    error       TEXT NOT NULL DEFAULT ''
) STRICT;

CREATE INDEX sync_runs_started ON sync_runs (started_at);

-- The one lock table every apply path uses (01 §5.1 step 3); key = subsystem connection, e.g. dns:<zone id>.
-- Hashi is one process: locks left by a crash are cleared at start.
CREATE TABLE sync_locks (
    key         TEXT PRIMARY KEY,
    holder      TEXT NOT NULL,
    acquired_at INTEGER NOT NULL
) STRICT;

-- +goose Down
DROP TABLE sync_locks;
DROP TABLE sync_runs;
DROP TABLE sync_plan_items;
DROP TABLE sync_plans;
DROP TABLE dns_records;
