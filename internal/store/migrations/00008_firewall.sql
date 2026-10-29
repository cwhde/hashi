-- +goose Up
-- S6: Linux firewall hosts (05-firewall.md), the global allow/block bindings and per-host access (05 §3.2, §4).

-- A firewall host (05 §1). Targets are literal until connection targets become address sets (08 §5); the internal
-- Traefik address is the DNAT target. Detected values come from the probe (testdata/contracts/firewall); a
-- non-empty override wins.
CREATE TABLE firewall_hosts (
    id                     TEXT PRIMARY KEY,
    name                   TEXT NOT NULL UNIQUE,
    edge_mode              TEXT NOT NULL CHECK (edge_mode IN ('managed', 'external_nat')),
    ssh_host               TEXT NOT NULL DEFAULT '',
    ssh_port               INTEGER NOT NULL DEFAULT 22 CHECK (ssh_port BETWEEN 1 AND 65535),
    ssh_user               TEXT NOT NULL DEFAULT '',
    auth                   TEXT NOT NULL DEFAULT 'hashi_key' CHECK (auth IN ('hashi_key', 'private_key', 'password')),
    credential_secret_id   TEXT REFERENCES secret_records (id),
    passphrase_secret_id   TEXT REFERENCES secret_records (id),
    host_key_type          TEXT NOT NULL DEFAULT '',
    host_key               BLOB,
    host_key_fingerprint   TEXT NOT NULL DEFAULT '',
    traefik_connection_id  TEXT REFERENCES traefik_connections (id) ON DELETE RESTRICT,
    internal_traefik_ip    TEXT NOT NULL DEFAULT '',
    -- The host whose via. record this host's on. record points at (03 §5.1); NULL = itself.
    route_via_host_id      TEXT REFERENCES firewall_hosts (id) ON DELETE RESTRICT,
    public_ip              TEXT NOT NULL DEFAULT '',
    public_ip6             TEXT NOT NULL DEFAULT '',
    public_ip_override     TEXT NOT NULL DEFAULT '',
    public_ip6_override    TEXT NOT NULL DEFAULT '',
    wan_iface              TEXT NOT NULL DEFAULT '',
    wan_iface_override     TEXT NOT NULL DEFAULT '',
    -- Managed subnets as a JSON array of prefixes.
    subnets_json           TEXT NOT NULL DEFAULT '[]',
    -- Addresses for host items with scope internal (NetBird and bridge addresses), JSON array.
    internal_ips_json      TEXT NOT NULL DEFAULT '[]',
    netbird_mode           TEXT NOT NULL DEFAULT 'disabled' CHECK (netbird_mode IN ('disabled', 'peer', 'routing')),
    netbird_iface          TEXT NOT NULL DEFAULT '',
    netbird_port           INTEGER NOT NULL DEFAULT 0,
    -- The user accepted "block all other inbound" for this host (05 §3.2 chain 4); NULL = not accepted.
    drop_inbound_accepted_at INTEGER,
    -- Probe facts (firewall.Facts) and the ruleset read at setup, shown before the first plan (05 §2).
    facts_json             TEXT NOT NULL DEFAULT '{}',
    setup_ruleset          TEXT NOT NULL DEFAULT '',
    ownership_confirmed_at INTEGER,
    health                 TEXT NOT NULL DEFAULT 'unknown'
                           CHECK (health IN ('unknown', 'validating', 'healthy', 'degraded', 'failed')),
    last_validated_at      INTEGER,
    last_validation_error  TEXT NOT NULL DEFAULT '',
    last_applied_hash      TEXT NOT NULL DEFAULT '',
    last_applied_at        INTEGER,
    created_at             INTEGER NOT NULL,
    updated_at             INTEGER NOT NULL,
    CHECK (edge_mode = 'external_nat' OR (ssh_host != '' AND ssh_user != '')),
    CHECK (auth = 'hashi_key' OR credential_secret_id IS NOT NULL),
    CHECK (route_via_host_id IS NULL OR route_via_host_id != id)
) STRICT;

-- Global firewall allow / block: lists of IP sets, folded into every managed host (05 §4.1).
CREATE TABLE firewall_bindings (
    position   TEXT NOT NULL CHECK (position IN ('allow', 'block')),
    ip_set_id  TEXT NOT NULL REFERENCES ip_sets (id) ON DELETE RESTRICT,
    ord        INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    PRIMARY KEY (position, ip_set_id)
) STRICT;
CREATE INDEX firewall_bindings_set ON firewall_bindings (ip_set_id);

-- Host access: sets allowed to reach the host itself (05 §3.2 chain 4).
CREATE TABLE firewall_host_access (
    host_id    TEXT NOT NULL REFERENCES firewall_hosts (id) ON DELETE CASCADE,
    ip_set_id  TEXT NOT NULL REFERENCES ip_sets (id) ON DELETE RESTRICT,
    created_at INTEGER NOT NULL,
    PRIMARY KEY (host_id, ip_set_id)
) STRICT;
CREATE INDEX firewall_host_access_set ON firewall_host_access (ip_set_id);

-- sync_plans.subsystem gains firewall (Hashi's table, boot unit and sysctl per managed host, 05 §3). SQLite cannot change a CHECK in place: the three sync tables are rebuilt. The children are copied
-- into new tables first and the old ones dropped children-first, so no ON DELETE CASCADE fires on the way.
CREATE TABLE sync_plans_v8 (
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
INSERT INTO sync_plans_v8 SELECT * FROM sync_plans;

CREATE TABLE sync_plan_items_v8 (
    plan_id     TEXT NOT NULL REFERENCES sync_plans_v8 (id) ON DELETE CASCADE,
    seq         INTEGER NOT NULL,
    action      TEXT NOT NULL CHECK (action IN ('create', 'update', 'delete', 'label', 'conflict')),
    object      TEXT NOT NULL,
    before_json TEXT NOT NULL DEFAULT '',
    after_json  TEXT NOT NULL DEFAULT '',
    risk        TEXT NOT NULL CHECK (risk IN ('low', 'high')),
    note        TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (plan_id, seq)
) STRICT;
INSERT INTO sync_plan_items_v8 SELECT * FROM sync_plan_items;

CREATE TABLE sync_runs_v8 (
    id          TEXT PRIMARY KEY,
    plan_id     TEXT NOT NULL REFERENCES sync_plans_v8 (id) ON DELETE CASCADE,
    started_at  INTEGER NOT NULL,
    finished_at INTEGER,
    outcome     TEXT NOT NULL DEFAULT 'running'
                CHECK (outcome IN ('running', 'applied', 'failed', 'replanned', 'no_changes')),
    summary     TEXT NOT NULL DEFAULT '',
    error       TEXT NOT NULL DEFAULT ''
) STRICT;
INSERT INTO sync_runs_v8 SELECT * FROM sync_runs;

DROP TABLE sync_plan_items;
DROP TABLE sync_runs;
DROP TABLE sync_plans;
-- Renaming the parent rewrites the children's REFERENCES to the new name.
ALTER TABLE sync_plans_v8 RENAME TO sync_plans;
ALTER TABLE sync_plan_items_v8 RENAME TO sync_plan_items;
ALTER TABLE sync_runs_v8 RENAME TO sync_runs;
CREATE UNIQUE INDEX sync_plans_open ON sync_plans (subsystem, target) WHERE status IN ('pending', 'approved', 'applying');
CREATE INDEX sync_plans_created ON sync_plans (created_at);
CREATE INDEX sync_runs_started ON sync_runs (started_at);


-- +goose Down
DELETE FROM sync_plans WHERE subsystem = 'firewall';
CREATE TABLE sync_plans_v8d (
    id          TEXT PRIMARY KEY,
    subsystem   TEXT NOT NULL CHECK (subsystem IN ('dns-records', 'dns-record-deletes', 'dns-prune', 'dns-probe',
                                                   'traefik', 'certificates')),
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
INSERT INTO sync_plans_v8d SELECT * FROM sync_plans;
CREATE TABLE sync_plan_items_v8d (
    plan_id     TEXT NOT NULL REFERENCES sync_plans_v8d (id) ON DELETE CASCADE,
    seq         INTEGER NOT NULL,
    action      TEXT NOT NULL CHECK (action IN ('create', 'update', 'delete', 'label', 'conflict')),
    object      TEXT NOT NULL,
    before_json TEXT NOT NULL DEFAULT '',
    after_json  TEXT NOT NULL DEFAULT '',
    risk        TEXT NOT NULL CHECK (risk IN ('low', 'high')),
    note        TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (plan_id, seq)
) STRICT;
INSERT INTO sync_plan_items_v8d SELECT * FROM sync_plan_items;
CREATE TABLE sync_runs_v8d (
    id          TEXT PRIMARY KEY,
    plan_id     TEXT NOT NULL REFERENCES sync_plans_v8d (id) ON DELETE CASCADE,
    started_at  INTEGER NOT NULL,
    finished_at INTEGER,
    outcome     TEXT NOT NULL DEFAULT 'running'
                CHECK (outcome IN ('running', 'applied', 'failed', 'replanned', 'no_changes')),
    summary     TEXT NOT NULL DEFAULT '',
    error       TEXT NOT NULL DEFAULT ''
) STRICT;
INSERT INTO sync_runs_v8d SELECT * FROM sync_runs;
DROP TABLE sync_plan_items;
DROP TABLE sync_runs;
DROP TABLE sync_plans;
ALTER TABLE sync_plans_v8d RENAME TO sync_plans;
ALTER TABLE sync_plan_items_v8d RENAME TO sync_plan_items;
ALTER TABLE sync_runs_v8d RENAME TO sync_runs;
CREATE UNIQUE INDEX sync_plans_open ON sync_plans (subsystem, target) WHERE status IN ('pending', 'approved', 'applying');
CREATE INDEX sync_plans_created ON sync_plans (created_at);
CREATE INDEX sync_runs_started ON sync_runs (started_at);

DROP TABLE firewall_host_access;
DROP TABLE firewall_bindings;
DROP TABLE firewall_hosts;
