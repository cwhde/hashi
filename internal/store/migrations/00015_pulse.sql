-- +goose Up
-- S8: Pulse agents (08 §2). token_hash is HMAC-SHA256(pepper, token); the pepper is in the vault, its record named here.
CREATE TABLE pulse_config (
    id               INTEGER PRIMARY KEY CHECK (id = 1),
    pepper_secret_id TEXT NOT NULL REFERENCES secret_records (id)
) STRICT;

CREATE TABLE pulse_agents (
    id               TEXT PRIMARY KEY,
    name             TEXT NOT NULL UNIQUE,
    token_hash       BLOB NOT NULL,
    install_type     TEXT NOT NULL DEFAULT 'linux' CHECK (install_type IN ('linux', 'docker')),
    interval_seconds INTEGER NOT NULL DEFAULT 60,
    last_seen_at     INTEGER,
    public_ip        TEXT NOT NULL DEFAULT '',
    candidates_json  TEXT NOT NULL DEFAULT '[]',
    selected_ip      TEXT NOT NULL DEFAULT '',
    hostname         TEXT NOT NULL DEFAULT '',
    version          TEXT NOT NULL DEFAULT '',
    revoked_at       INTEGER,
    dns_enabled      INTEGER NOT NULL DEFAULT 1 CHECK (dns_enabled IN (0, 1)),
    dns_name         TEXT NOT NULL DEFAULT '',
    created_at       INTEGER NOT NULL,
    updated_at       INTEGER NOT NULL
) STRICT;

-- +goose Down
DROP TABLE pulse_agents;
DROP TABLE pulse_config;
