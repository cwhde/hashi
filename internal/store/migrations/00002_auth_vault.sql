-- +goose Up
-- S1: setup state machine, audit log, admin auth, sessions, secrets vault (02-setup-auth.md).
-- Timestamps are integer Unix milliseconds UTC (invariant 6), IDs are ULID text unless stated.

CREATE TABLE setup_state (
    step        INTEGER PRIMARY KEY CHECK (step BETWEEN 1 AND 11),
    status      TEXT NOT NULL CHECK (status IN ('pending', 'in_progress', 'done', 'failed')),
    inputs_json TEXT NOT NULL DEFAULT '{}',
    last_error  TEXT NOT NULL DEFAULT '',
    updated_at  INTEGER NOT NULL
) STRICT;

CREATE TABLE audit_events (
    id          TEXT PRIMARY KEY,
    at          INTEGER NOT NULL,
    -- 'session:<salted hash prefix>', 'system', 'console' or 'anonymous'. Never a raw token.
    actor       TEXT NOT NULL,
    action      TEXT NOT NULL,
    target      TEXT NOT NULL DEFAULT '',
    outcome     TEXT NOT NULL CHECK (outcome IN ('success', 'failure', 'denied')),
    ip          TEXT NOT NULL DEFAULT '',
    detail_json TEXT NOT NULL DEFAULT '{}'
) STRICT;

CREATE INDEX audit_events_at ON audit_events (at DESC, id DESC);

-- Exactly one admin (02 §2). user_handle is the WebAuthn user handle (random bytes).
CREATE TABLE admin_users (
    id          INTEGER PRIMARY KEY CHECK (id = 1),
    user_handle BLOB NOT NULL,
    created_at  INTEGER NOT NULL
) STRICT;

-- Bootstrap credentials (02 §1.1): argon2id hash only; deleted at setup step 11.
CREATE TABLE bootstrap_credentials (
    id            INTEGER PRIMARY KEY CHECK (id = 1),
    username      TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    created_at    INTEGER NOT NULL
) STRICT;

CREATE TABLE passkey_credentials (
    id              TEXT PRIMARY KEY,
    credential_id   BLOB NOT NULL UNIQUE,
    -- go-webauthn Credential as JSON (public key, sign count, flags, transports, AAGUID). No secret material.
    credential_json TEXT NOT NULL,
    name            TEXT NOT NULL,
    -- RP ID (root domain) the credential is bound to; a root-domain change makes it unusable (02 §2).
    rp_id           TEXT NOT NULL,
    aaguid_label    TEXT NOT NULL DEFAULT '',
    created_at      INTEGER NOT NULL,
    last_used_at    INTEGER
) STRICT;

-- id_hash = sha256(session id); the raw id only ever lives in the cookie (02 §3.1).
CREATE TABLE admin_sessions (
    id_hash             BLOB PRIMARY KEY,
    auth_method         TEXT NOT NULL CHECK (auth_method IN ('bootstrap', 'passkey', 'recovery')),
    credential_id       TEXT REFERENCES passkey_credentials (id) ON DELETE SET NULL,
    bound_ip            TEXT NOT NULL,
    scopes_json         TEXT NOT NULL,
    created_at          INTEGER NOT NULL,
    last_seen_at        INTEGER NOT NULL,
    idle_expires_at     INTEGER NOT NULL,
    absolute_expires_at INTEGER NOT NULL,
    reauth_at           INTEGER,
    revoked_at          INTEGER,
    revocation_reason   TEXT,
    user_agent_hash     TEXT NOT NULL DEFAULT '',
    CHECK (idle_expires_at <= absolute_expires_at)
) STRICT;

CREATE INDEX admin_sessions_expiry ON admin_sessions (absolute_expires_at);

-- Envelope-encrypted secrets (02 §5.2). Payload sealed with a per-secret DEK; DEK wrapped with the class KEK.
CREATE TABLE secret_records (
    id          TEXT PRIMARY KEY,
    class       TEXT NOT NULL CHECK (class IN ('service', 'admin')),
    purpose     TEXT NOT NULL,
    key_version INTEGER NOT NULL,
    nonce       BLOB NOT NULL,
    ciphertext  BLOB NOT NULL,
    dek_nonce   BLOB NOT NULL,
    wrapped_dek BLOB NOT NULL,
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL
) STRICT;

-- Recovery key wraps a copy of the master key (02 §5.3). verifier lets the console check an entered key.
CREATE TABLE recovery_key (
    id             INTEGER PRIMARY KEY CHECK (id = 1),
    key_version    INTEGER NOT NULL,
    verifier       BLOB NOT NULL,
    nonce          BLOB NOT NULL,
    wrapped_master BLOB NOT NULL,
    created_at     INTEGER NOT NULL,
    confirmed_at   INTEGER
) STRICT;

-- One-time console recovery links (`hashi admin recover`, 02 §2): hash only, 15 minutes, single use.
CREATE TABLE recovery_links (
    token_hash BLOB PRIMARY KEY,
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL,
    used_at    INTEGER
) STRICT;

-- The S0 proof job is replaced by real jobs.
DELETE FROM jobs WHERE kind = 'noop';

-- +goose Down
DROP TABLE recovery_links;
DROP TABLE recovery_key;
DROP TABLE secret_records;
DROP INDEX admin_sessions_expiry;
DROP TABLE admin_sessions;
DROP TABLE passkey_credentials;
DROP TABLE bootstrap_credentials;
DROP TABLE admin_users;
DROP INDEX audit_events_at;
DROP TABLE audit_events;
DROP TABLE setup_state;
