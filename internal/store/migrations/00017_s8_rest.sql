-- +goose Up
-- The rest of S8. Pulse: optional reachability port (08 §2). Hosts: SSH through an address set (08 §5), the literal stays
-- as the fallback and the pinned host key still decides who is who. Monitoring gains AdGuard and Pulse sources (07 §1).
ALTER TABLE pulse_agents ADD COLUMN reach_port INTEGER NOT NULL DEFAULT 0 CHECK (reach_port BETWEEN 0 AND 65535);
ALTER TABLE pulse_agents ADD COLUMN reach_ok INTEGER CHECK (reach_ok IN (0, 1));
ALTER TABLE firewall_hosts ADD COLUMN ssh_target_set_id TEXT REFERENCES ip_sets (id) ON DELETE RESTRICT;
ALTER TABLE traefik_connections ADD COLUMN ssh_target_set_id TEXT REFERENCES ip_sets (id) ON DELETE RESTRICT;

-- SQLite cannot change a CHECK in place: the endpoint table and the tables pointing at it are rebuilt, as in 00008.
CREATE TABLE monitor_endpoints_v17 (
    id               TEXT PRIMARY KEY,
    source_type      TEXT NOT NULL CHECK (source_type IN ('service', 'dns_record', 'firewall_host', 'traefik', 'hashi', 'user', 'adguard', 'pulse')),
    source_id        TEXT NOT NULL DEFAULT '',
    name             TEXT NOT NULL,
    group_name       TEXT NOT NULL,
    paused           INTEGER NOT NULL DEFAULT 0 CHECK (paused IN (0, 1)),
    interval_seconds INTEGER NOT NULL DEFAULT 60 CHECK (interval_seconds BETWEEN 10 AND 86400),
    timeout_seconds  INTEGER NOT NULL DEFAULT 10 CHECK (timeout_seconds BETWEEN 1 AND 60),
    degraded_ms      INTEGER NOT NULL DEFAULT 0 CHECK (degraded_ms >= 0), -- 0: no latency threshold
    public           INTEGER NOT NULL DEFAULT 0 CHECK (public IN (0, 1)),
    route_provider_id TEXT REFERENCES notification_providers (id) ON DELETE SET NULL,
    state            TEXT NOT NULL DEFAULT 'unknown' CHECK (state IN ('up', 'degraded', 'down', 'unknown')),
    state_since      INTEGER NOT NULL,
    good_rounds      INTEGER NOT NULL DEFAULT 0,
    bad_rounds       INTEGER NOT NULL DEFAULT 0,
    last_check_at    INTEGER,
    last_latency_ms  INTEGER,
    created_at       INTEGER NOT NULL,
    updated_at       INTEGER NOT NULL,
    UNIQUE (source_type, source_id, name)
) STRICT;
INSERT INTO monitor_endpoints_v17 SELECT * FROM monitor_endpoints;

CREATE TABLE monitor_checks_v17 (
    id            TEXT PRIMARY KEY,
    endpoint_id   TEXT NOT NULL REFERENCES monitor_endpoints_v17 (id) ON DELETE CASCADE,
    type          TEXT NOT NULL CHECK (type IN ('http', 'https', 'h2c', 'tcp', 'udp', 'dns', 'tls', 'self')),
    role          TEXT NOT NULL DEFAULT '' CHECK (role IN ('', 'public', 'internal')),
    target        TEXT NOT NULL,
    required      INTEGER NOT NULL DEFAULT 0 CHECK (required IN (0, 1)),
    code_min      INTEGER NOT NULL DEFAULT 200,
    code_max      INTEGER NOT NULL DEFAULT 399,
    skip_verify   INTEGER NOT NULL DEFAULT 0 CHECK (skip_verify IN (0, 1)),
    probe         TEXT NOT NULL DEFAULT '', -- udp: payload to send; dns: record type
    expect        TEXT NOT NULL DEFAULT '', -- udp: expected response; dns: expected answer
    UNIQUE (endpoint_id, type, role, target)
) STRICT;
INSERT INTO monitor_checks_v17 SELECT * FROM monitor_checks;

CREATE TABLE monitor_samples_v17 (
    endpoint_id TEXT NOT NULL REFERENCES monitor_endpoints_v17 (id) ON DELETE CASCADE,
    check_id    TEXT NOT NULL,
    at          INTEGER NOT NULL,
    verdict     TEXT NOT NULL CHECK (verdict IN ('up', 'degraded', 'down')),
    ok          INTEGER NOT NULL CHECK (ok IN (0, 1)),
    latency_ms  INTEGER NOT NULL,
    detail      TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (endpoint_id, check_id, at)
) STRICT, WITHOUT ROWID;
INSERT INTO monitor_samples_v17 SELECT * FROM monitor_samples;

CREATE TABLE monitor_rollups_v17 (
    endpoint_id TEXT NOT NULL REFERENCES monitor_endpoints_v17 (id) ON DELETE CASCADE,
    check_id    TEXT NOT NULL,
    resolution  INTEGER NOT NULL CHECK (resolution IN (60, 300, 3600)),
    bucket      INTEGER NOT NULL,
    count       INTEGER NOT NULL,
    ok_count    INTEGER NOT NULL,
    min_ms      INTEGER NOT NULL,
    max_ms      INTEGER NOT NULL,
    avg_ms      INTEGER NOT NULL,
    p95_ms      INTEGER NOT NULL,
    PRIMARY KEY (endpoint_id, check_id, resolution, bucket)
) STRICT, WITHOUT ROWID;
INSERT INTO monitor_rollups_v17 SELECT * FROM monitor_rollups;

CREATE TABLE monitor_events_v17 (
    id          TEXT PRIMARY KEY,
    endpoint_id TEXT NOT NULL REFERENCES monitor_endpoints_v17 (id) ON DELETE CASCADE,
    at          INTEGER NOT NULL,
    from_state  TEXT NOT NULL,
    to_state    TEXT NOT NULL,
    message     TEXT NOT NULL
) STRICT;
INSERT INTO monitor_events_v17 SELECT * FROM monitor_events;

CREATE TABLE notification_deliveries_v17 (
    id              TEXT PRIMARY KEY,
    provider_id     TEXT NOT NULL REFERENCES notification_providers (id) ON DELETE CASCADE,
    endpoint_id     TEXT REFERENCES monitor_endpoints_v17 (id) ON DELETE SET NULL,
    kind            TEXT NOT NULL CHECK (kind IN ('alert', 'recovery', 'test')),
    subject         TEXT NOT NULL,
    body            TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'delivered', 'failed')),
    attempts        INTEGER NOT NULL DEFAULT 0,
    next_attempt_at INTEGER NOT NULL,
    last_error      TEXT NOT NULL DEFAULT '',
    created_at      INTEGER NOT NULL,
    delivered_at    INTEGER
) STRICT;
INSERT INTO notification_deliveries_v17 SELECT * FROM notification_deliveries;

DROP TABLE notification_deliveries;
DROP TABLE monitor_events;
DROP TABLE monitor_rollups;
DROP TABLE monitor_samples;
DROP TABLE monitor_checks;
DROP TABLE monitor_endpoints;
ALTER TABLE monitor_endpoints_v17 RENAME TO monitor_endpoints;
ALTER TABLE monitor_checks_v17 RENAME TO monitor_checks;
ALTER TABLE monitor_samples_v17 RENAME TO monitor_samples;
ALTER TABLE monitor_rollups_v17 RENAME TO monitor_rollups;
ALTER TABLE monitor_events_v17 RENAME TO monitor_events;
ALTER TABLE notification_deliveries_v17 RENAME TO notification_deliveries;
CREATE INDEX monitor_checks_endpoint ON monitor_checks (endpoint_id);
CREATE INDEX monitor_samples_at ON monitor_samples (at);
CREATE INDEX monitor_rollups_bucket ON monitor_rollups (resolution, bucket);
CREATE INDEX monitor_events_endpoint ON monitor_events (endpoint_id, at);
CREATE INDEX notification_deliveries_due ON notification_deliveries (status, next_attempt_at);
CREATE INDEX notification_deliveries_endpoint ON notification_deliveries (endpoint_id, created_at);

-- An address changed and DNS may not have caught up; cleared by the next successful check (08 §4 step 6).
ALTER TABLE monitor_endpoints ADD COLUMN pending_dns INTEGER NOT NULL DEFAULT 0 CHECK (pending_dns IN (0, 1));

-- +goose Down
ALTER TABLE monitor_endpoints DROP COLUMN pending_dns;
DELETE FROM monitor_endpoints WHERE source_type IN ('adguard', 'pulse');
CREATE TABLE monitor_endpoints_v17d (
    id               TEXT PRIMARY KEY,
    source_type      TEXT NOT NULL CHECK (source_type IN ('service', 'dns_record', 'firewall_host', 'traefik', 'hashi', 'user')),
    source_id        TEXT NOT NULL DEFAULT '',
    name             TEXT NOT NULL,
    group_name       TEXT NOT NULL,
    paused           INTEGER NOT NULL DEFAULT 0 CHECK (paused IN (0, 1)),
    interval_seconds INTEGER NOT NULL DEFAULT 60 CHECK (interval_seconds BETWEEN 10 AND 86400),
    timeout_seconds  INTEGER NOT NULL DEFAULT 10 CHECK (timeout_seconds BETWEEN 1 AND 60),
    degraded_ms      INTEGER NOT NULL DEFAULT 0 CHECK (degraded_ms >= 0), -- 0: no latency threshold
    public           INTEGER NOT NULL DEFAULT 0 CHECK (public IN (0, 1)),
    route_provider_id TEXT REFERENCES notification_providers (id) ON DELETE SET NULL,
    state            TEXT NOT NULL DEFAULT 'unknown' CHECK (state IN ('up', 'degraded', 'down', 'unknown')),
    state_since      INTEGER NOT NULL,
    good_rounds      INTEGER NOT NULL DEFAULT 0,
    bad_rounds       INTEGER NOT NULL DEFAULT 0,
    last_check_at    INTEGER,
    last_latency_ms  INTEGER,
    created_at       INTEGER NOT NULL,
    updated_at       INTEGER NOT NULL,
    UNIQUE (source_type, source_id, name)
) STRICT;
INSERT INTO monitor_endpoints_v17d SELECT * FROM monitor_endpoints;

CREATE TABLE monitor_checks_v17d (
    id            TEXT PRIMARY KEY,
    endpoint_id   TEXT NOT NULL REFERENCES monitor_endpoints_v17d (id) ON DELETE CASCADE,
    type          TEXT NOT NULL CHECK (type IN ('http', 'https', 'h2c', 'tcp', 'udp', 'dns', 'tls', 'self')),
    role          TEXT NOT NULL DEFAULT '' CHECK (role IN ('', 'public', 'internal')),
    target        TEXT NOT NULL,
    required      INTEGER NOT NULL DEFAULT 0 CHECK (required IN (0, 1)),
    code_min      INTEGER NOT NULL DEFAULT 200,
    code_max      INTEGER NOT NULL DEFAULT 399,
    skip_verify   INTEGER NOT NULL DEFAULT 0 CHECK (skip_verify IN (0, 1)),
    probe         TEXT NOT NULL DEFAULT '', -- udp: payload to send; dns: record type
    expect        TEXT NOT NULL DEFAULT '', -- udp: expected response; dns: expected answer
    UNIQUE (endpoint_id, type, role, target)
) STRICT;
INSERT INTO monitor_checks_v17d SELECT * FROM monitor_checks;

CREATE TABLE monitor_samples_v17d (
    endpoint_id TEXT NOT NULL REFERENCES monitor_endpoints_v17d (id) ON DELETE CASCADE,
    check_id    TEXT NOT NULL,
    at          INTEGER NOT NULL,
    verdict     TEXT NOT NULL CHECK (verdict IN ('up', 'degraded', 'down')),
    ok          INTEGER NOT NULL CHECK (ok IN (0, 1)),
    latency_ms  INTEGER NOT NULL,
    detail      TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (endpoint_id, check_id, at)
) STRICT, WITHOUT ROWID;
INSERT INTO monitor_samples_v17d SELECT * FROM monitor_samples;

CREATE TABLE monitor_rollups_v17d (
    endpoint_id TEXT NOT NULL REFERENCES monitor_endpoints_v17d (id) ON DELETE CASCADE,
    check_id    TEXT NOT NULL,
    resolution  INTEGER NOT NULL CHECK (resolution IN (60, 300, 3600)),
    bucket      INTEGER NOT NULL,
    count       INTEGER NOT NULL,
    ok_count    INTEGER NOT NULL,
    min_ms      INTEGER NOT NULL,
    max_ms      INTEGER NOT NULL,
    avg_ms      INTEGER NOT NULL,
    p95_ms      INTEGER NOT NULL,
    PRIMARY KEY (endpoint_id, check_id, resolution, bucket)
) STRICT, WITHOUT ROWID;
INSERT INTO monitor_rollups_v17d SELECT * FROM monitor_rollups;

CREATE TABLE monitor_events_v17d (
    id          TEXT PRIMARY KEY,
    endpoint_id TEXT NOT NULL REFERENCES monitor_endpoints_v17d (id) ON DELETE CASCADE,
    at          INTEGER NOT NULL,
    from_state  TEXT NOT NULL,
    to_state    TEXT NOT NULL,
    message     TEXT NOT NULL
) STRICT;
INSERT INTO monitor_events_v17d SELECT * FROM monitor_events;

CREATE TABLE notification_deliveries_v17d (
    id              TEXT PRIMARY KEY,
    provider_id     TEXT NOT NULL REFERENCES notification_providers (id) ON DELETE CASCADE,
    endpoint_id     TEXT REFERENCES monitor_endpoints_v17d (id) ON DELETE SET NULL,
    kind            TEXT NOT NULL CHECK (kind IN ('alert', 'recovery', 'test')),
    subject         TEXT NOT NULL,
    body            TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'delivered', 'failed')),
    attempts        INTEGER NOT NULL DEFAULT 0,
    next_attempt_at INTEGER NOT NULL,
    last_error      TEXT NOT NULL DEFAULT '',
    created_at      INTEGER NOT NULL,
    delivered_at    INTEGER
) STRICT;
INSERT INTO notification_deliveries_v17d SELECT * FROM notification_deliveries;

DROP TABLE notification_deliveries;
DROP TABLE monitor_events;
DROP TABLE monitor_rollups;
DROP TABLE monitor_samples;
DROP TABLE monitor_checks;
DROP TABLE monitor_endpoints;
ALTER TABLE monitor_endpoints_v17d RENAME TO monitor_endpoints;
ALTER TABLE monitor_checks_v17d RENAME TO monitor_checks;
ALTER TABLE monitor_samples_v17d RENAME TO monitor_samples;
ALTER TABLE monitor_rollups_v17d RENAME TO monitor_rollups;
ALTER TABLE monitor_events_v17d RENAME TO monitor_events;
ALTER TABLE notification_deliveries_v17d RENAME TO notification_deliveries;
CREATE INDEX monitor_checks_endpoint ON monitor_checks (endpoint_id);
CREATE INDEX monitor_samples_at ON monitor_samples (at);
CREATE INDEX monitor_rollups_bucket ON monitor_rollups (resolution, bucket);
CREATE INDEX monitor_events_endpoint ON monitor_events (endpoint_id, at);
CREATE INDEX notification_deliveries_due ON notification_deliveries (status, next_attempt_at);
CREATE INDEX notification_deliveries_endpoint ON notification_deliveries (endpoint_id, created_at);
ALTER TABLE traefik_connections DROP COLUMN ssh_target_set_id;
ALTER TABLE firewall_hosts DROP COLUMN ssh_target_set_id;
ALTER TABLE pulse_agents DROP COLUMN reach_ok;
ALTER TABLE pulse_agents DROP COLUMN reach_port;
