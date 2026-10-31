-- +goose Up
-- S7: monitoring and notifications (07 §1, §4, §6).

-- config_json holds everything but the secret; the password is in the vault (07 §6, invariant 5).
CREATE TABLE notification_providers (
    id          TEXT PRIMARY KEY,
    type        TEXT NOT NULL CHECK (type IN ('smtp', 'telegram')),
    name        TEXT NOT NULL UNIQUE,
    config_json TEXT NOT NULL,
    secret_id   TEXT REFERENCES secret_records (id) ON DELETE SET NULL,
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL
) STRICT;

-- source_type/source_id link an endpoint to what it watches; "user" endpoints have no source. A source's endpoint is
-- synced from it (name, checks), while group, schedule, visibility and the route override stay the admin's.
CREATE TABLE monitor_endpoints (
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

-- role names the path of an HTTP check: 'public' through DNS and Traefik, 'internal' to the upstream (07 §2).
CREATE TABLE monitor_checks (
    id            TEXT PRIMARY KEY,
    endpoint_id   TEXT NOT NULL REFERENCES monitor_endpoints (id) ON DELETE CASCADE,
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
CREATE INDEX monitor_checks_endpoint ON monitor_checks (endpoint_id);

-- check_id '' is the endpoint's round: the verdict all its checks gave together. ok: 1 unless the verdict is down.
CREATE TABLE monitor_samples (
    endpoint_id TEXT NOT NULL REFERENCES monitor_endpoints (id) ON DELETE CASCADE,
    check_id    TEXT NOT NULL,
    at          INTEGER NOT NULL,
    verdict     TEXT NOT NULL CHECK (verdict IN ('up', 'degraded', 'down')),
    ok          INTEGER NOT NULL CHECK (ok IN (0, 1)),
    latency_ms  INTEGER NOT NULL,
    detail      TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (endpoint_id, check_id, at)
) STRICT, WITHOUT ROWID;
CREATE INDEX monitor_samples_at ON monitor_samples (at);

-- Written only for closed buckets (07 §4). resolution in seconds: 60, 300, 3600.
CREATE TABLE monitor_rollups (
    endpoint_id TEXT NOT NULL REFERENCES monitor_endpoints (id) ON DELETE CASCADE,
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
CREATE INDEX monitor_rollups_bucket ON monitor_rollups (resolution, bucket);

CREATE TABLE monitor_events (
    id          TEXT PRIMARY KEY,
    endpoint_id TEXT NOT NULL REFERENCES monitor_endpoints (id) ON DELETE CASCADE,
    at          INTEGER NOT NULL,
    from_state  TEXT NOT NULL,
    to_state    TEXT NOT NULL,
    message     TEXT NOT NULL
) STRICT;
CREATE INDEX monitor_events_endpoint ON monitor_events (endpoint_id, at);

-- The single default route; endpoints may override its provider (monitor_endpoints.route_provider_id).
CREATE TABLE notification_route (
    id               INTEGER PRIMARY KEY CHECK (id = 1),
    provider_id      TEXT REFERENCES notification_providers (id) ON DELETE SET NULL,
    min_severity     TEXT NOT NULL DEFAULT 'down' CHECK (min_severity IN ('degraded', 'down')),
    cooldown_seconds INTEGER NOT NULL DEFAULT 600 CHECK (cooldown_seconds >= 0),
    recovery         INTEGER NOT NULL DEFAULT 1 CHECK (recovery IN (0, 1)),
    updated_at       INTEGER NOT NULL
) STRICT;
INSERT INTO notification_route (id, updated_at) VALUES (1, 0);

CREATE TABLE notification_deliveries (
    id              TEXT PRIMARY KEY,
    provider_id     TEXT NOT NULL REFERENCES notification_providers (id) ON DELETE CASCADE,
    endpoint_id     TEXT REFERENCES monitor_endpoints (id) ON DELETE SET NULL,
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
CREATE INDEX notification_deliveries_due ON notification_deliveries (status, next_attempt_at);
CREATE INDEX notification_deliveries_endpoint ON notification_deliveries (endpoint_id, created_at);

-- +goose Down
DROP TABLE notification_deliveries;
DROP TABLE notification_route;
DROP TABLE monitor_events;
DROP TABLE monitor_rollups;
DROP TABLE monitor_samples;
DROP TABLE monitor_checks;
DROP TABLE monitor_endpoints;
DROP TABLE notification_providers;
