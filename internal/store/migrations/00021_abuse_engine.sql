-- +goose Up
-- Stage A's two dynamic sets (06 §3.5). Their entries are ordinary elements with origin 'engine'; what only the engine
-- needs lives in the side tables below (06a §2.3).
INSERT INTO ip_sets (id, slug, name, description, kind, origin, system_key, created_at, updated_at) VALUES
    ('sys-dynamic-challenge', 'dynamic-challenge', 'dynamic-challenge',
        'Addresses that must solve a CAPTCHA before anything else (abuse engine).', 'dynamic', 'system',
        'dynamic_challenge', unixepoch() * 1000, unixepoch() * 1000),
    ('sys-dynamic-blocked', 'dynamic-blocked', 'dynamic-blocked',
        'Addresses banned on every firewall host (abuse engine).', 'dynamic', 'system', 'dynamic_blocked',
        unixepoch() * 1000, unixepoch() * 1000);

-- One row per element of a dynamic set. Ended entries keep their row (removed_at) until the element is deleted.
CREATE TABLE dynamic_entry_state (
    element_id       TEXT PRIMARY KEY REFERENCES ip_set_elements (id) ON DELETE CASCADE,
    source           TEXT NOT NULL CHECK (source IN ('l7_rate', 'l7_waf', 'l7_challenge_failed', 'l4_meter', 'manual')),
    meter            TEXT NOT NULL DEFAULT '',
    firewall_host_id TEXT REFERENCES firewall_hosts (id) ON DELETE SET NULL,
    service_id       TEXT REFERENCES services (id) ON DELETE SET NULL,
    reason           TEXT NOT NULL DEFAULT '',
    failure_count    INTEGER NOT NULL DEFAULT 0,
    removed_at       INTEGER,
    removed_reason   TEXT NOT NULL DEFAULT ''
) STRICT;

-- Where a ban stands on each managed firewall host (05 §6 step 4).
CREATE TABLE dynamic_entry_hosts (
    element_id       TEXT NOT NULL REFERENCES ip_set_elements (id) ON DELETE CASCADE,
    firewall_host_id TEXT NOT NULL REFERENCES firewall_hosts (id) ON DELETE CASCADE,
    state            TEXT NOT NULL CHECK (state IN ('pending', 'applied', 'removed', 'failed')),
    applied_at       INTEGER,
    error            TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (element_id, firewall_host_id)
) STRICT;

-- Ban and challenge counters per address (IPv6 per /64), 06 §5.1. Counts are as of their quiet-since time; the
-- effective value decays by one per ban.counter_decay_ttl and is computed on read.
CREATE TABLE address_counters (
    address               TEXT PRIMARY KEY,
    ban_count             INTEGER NOT NULL DEFAULT 0,
    ban_quiet_since       INTEGER NOT NULL DEFAULT 0,
    challenge_count       INTEGER NOT NULL DEFAULT 0,
    challenge_quiet_since INTEGER NOT NULL DEFAULT 0,
    last_ban_at           INTEGER,
    last_ban_ends_at      INTEGER,
    last_challenge_at     INTEGER,
    last_source           TEXT NOT NULL DEFAULT '',
    country               TEXT NOT NULL DEFAULT '',
    asn                   INTEGER NOT NULL DEFAULT 0,
    created_at            INTEGER NOT NULL,
    updated_at            INTEGER NOT NULL
) STRICT;
CREATE INDEX address_counters_ban_quiet ON address_counters (ban_quiet_since);
CREATE INDEX address_counters_challenge_quiet ON address_counters (challenge_quiet_since);
CREATE INDEX address_counters_last_ban ON address_counters (last_ban_at);
CREATE INDEX address_counters_last_challenge ON address_counters (last_challenge_at);

-- Telemetry (06 §6.1, 06a §6): non-allow decisions and state changes, at most 10 per address per minute.
CREATE TABLE security_events (
    id                    TEXT PRIMARY KEY,
    occurred_at           INTEGER NOT NULL,
    address               TEXT NOT NULL DEFAULT '',
    service_id            TEXT REFERENCES services (id) ON DELETE SET NULL,
    firewall_host_id      TEXT REFERENCES firewall_hosts (id) ON DELETE SET NULL,
    traefik_connection_id TEXT REFERENCES traefik_connections (id) ON DELETE SET NULL,
    event_type            TEXT NOT NULL,
    severity              TEXT NOT NULL CHECK (severity IN ('info', 'warning', 'critical')),
    decision              TEXT NOT NULL DEFAULT '',
    source                TEXT NOT NULL DEFAULT '',
    reason                TEXT NOT NULL DEFAULT '',
    request_method        TEXT NOT NULL DEFAULT '',
    request_path          TEXT NOT NULL DEFAULT '',
    status_code           INTEGER NOT NULL DEFAULT 0,
    user_agent_hash       TEXT NOT NULL DEFAULT '',
    request_id            TEXT NOT NULL DEFAULT '',
    suppressed            INTEGER NOT NULL DEFAULT 0,
    metadata_json         TEXT NOT NULL DEFAULT '{}'
) STRICT;
CREATE INDEX security_events_address ON security_events (address, occurred_at);
CREATE INDEX security_events_service ON security_events (service_id, occurred_at);
CREATE INDEX security_events_type ON security_events (event_type, occurred_at);
CREATE INDEX security_events_severity ON security_events (severity, occurred_at);

-- Closed minutes without addresses (volume bound, 01 §4.3).
CREATE TABLE security_buckets_1m (
    bucket_start           INTEGER NOT NULL,
    service_id             TEXT NOT NULL DEFAULT '',
    root_domain            TEXT NOT NULL DEFAULT '',
    traefik_connection_id  TEXT NOT NULL DEFAULT '',
    country                TEXT NOT NULL DEFAULT '',
    asn                    INTEGER NOT NULL DEFAULT 0,
    decision               TEXT NOT NULL,
    status_class           TEXT NOT NULL DEFAULT '',
    path_prefix            TEXT NOT NULL DEFAULT '',
    request_count          INTEGER NOT NULL DEFAULT 0,
    denied_count           INTEGER NOT NULL DEFAULT 0,
    challenged_count       INTEGER NOT NULL DEFAULT 0,
    challenge_failed_count INTEGER NOT NULL DEFAULT 0,
    banned_count           INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (bucket_start, service_id, root_domain, traefik_connection_id, country, asn, decision, status_class,
        path_prefix)
) STRICT, WITHOUT ROWID;

CREATE TABLE security_ip_top_1m (
    bucket_start     INTEGER NOT NULL,
    address          TEXT NOT NULL,
    service_id       TEXT NOT NULL DEFAULT '',
    request_count    INTEGER NOT NULL DEFAULT 0,
    denied_count     INTEGER NOT NULL DEFAULT 0,
    challenged_count INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (bucket_start, address, service_id)
) STRICT, WITHOUT ROWID;
CREATE INDEX security_ip_top_address ON security_ip_top_1m (address, bucket_start);

-- The persistent SSH session per managed firewall host (05 §6).
CREATE TABLE firewall_host_link (
    firewall_host_id TEXT PRIMARY KEY REFERENCES firewall_hosts (id) ON DELETE CASCADE,
    state            TEXT NOT NULL CHECK (state IN ('connecting', 'connected', 'down')),
    last_poll_at     INTEGER,
    last_push_at     INTEGER,
    lag_ms           INTEGER NOT NULL DEFAULT 0,
    error            TEXT NOT NULL DEFAULT '',
    updated_at       INTEGER NOT NULL
) STRICT;

-- +goose Down
DROP TABLE firewall_host_link;
DROP TABLE security_ip_top_1m;
DROP TABLE security_buckets_1m;
DROP TABLE security_events;
DROP TABLE address_counters;
DROP TABLE dynamic_entry_hosts;
DROP TABLE dynamic_entry_state;
DELETE FROM ip_set_elements WHERE ip_set_id IN ('sys-dynamic-challenge', 'sys-dynamic-blocked');
DELETE FROM ip_sets WHERE id IN ('sys-dynamic-challenge', 'sys-dynamic-blocked');
