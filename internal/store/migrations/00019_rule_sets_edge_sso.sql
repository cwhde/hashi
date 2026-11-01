-- +goose Up
-- Stage B (06 §4, 06a §3): one type for reusable sets, the global set and every service's root. A root belongs to its
-- service through service_id and goes with it.
CREATE TABLE rule_sets (
    id          TEXT PRIMARY KEY,
    slug        TEXT NOT NULL UNIQUE,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    kind        TEXT NOT NULL CHECK (kind IN ('user', 'global', 'service_root')),
    origin      TEXT NOT NULL CHECK (origin IN ('system', 'managed', 'imported', 'user')),
    service_id  TEXT UNIQUE REFERENCES services (id) ON DELETE CASCADE,
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL,
    CHECK ((kind = 'service_root') = (service_id IS NOT NULL))
) STRICT;
CREATE UNIQUE INDEX rule_sets_one_global ON rule_sets (kind) WHERE kind = 'global';
INSERT INTO rule_sets (id, slug, name, description, kind, origin, created_at, updated_at)
VALUES ('global', 'global', 'Global rules', 'Applies to every service after its own rules.', 'global', 'system',
    unixepoch() * 1000, unixepoch() * 1000);

-- Edge sign-in (06 §7).
CREATE TABLE oidc_providers (
    id             TEXT PRIMARY KEY,
    name           TEXT NOT NULL UNIQUE,
    issuer         TEXT NOT NULL,
    client_id      TEXT NOT NULL,
    secret_id      TEXT NOT NULL REFERENCES secret_records (id) ON DELETE RESTRICT,
    scopes         TEXT NOT NULL,
    username_claim TEXT NOT NULL,
    email_claim    TEXT NOT NULL,
    groups_claim   TEXT NOT NULL,
    is_default     INTEGER NOT NULL DEFAULT 0 CHECK (is_default IN (0, 1)),
    created_at     INTEGER NOT NULL,
    updated_at     INTEGER NOT NULL
) STRICT;
CREATE UNIQUE INDEX oidc_providers_one_default ON oidc_providers (is_default) WHERE is_default = 1;

CREATE TABLE rule_entries (
    id                TEXT PRIMARY KEY,
    rule_set_id       TEXT NOT NULL REFERENCES rule_sets (id) ON DELETE CASCADE,
    priority          INTEGER NOT NULL,
    entry_type        TEXT NOT NULL CHECK (entry_type IN ('item', 'set')),
    child_rule_set_id TEXT REFERENCES rule_sets (id) ON DELETE RESTRICT,
    applies_to_set_id TEXT REFERENCES ip_sets (id) ON DELETE RESTRICT,
    match_type        TEXT CHECK (match_type IN ('exact', 'prefix', 'regex')),
    pattern           TEXT,
    action            TEXT CHECK (action IN ('none', 'deny', 'pass_captcha', 'pass_authentication')),
    auth_provider_id  TEXT REFERENCES oidc_providers (id) ON DELETE RESTRICT,
    auth_groups_json  TEXT NOT NULL DEFAULT '[]',
    origin            TEXT NOT NULL CHECK (origin IN ('system', 'managed', 'imported', 'user')),
    locked            INTEGER NOT NULL DEFAULT 0 CHECK (locked IN (0, 1)),
    is_fallback       INTEGER NOT NULL DEFAULT 0 CHECK (is_fallback IN (0, 1)),
    enabled           INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    note              TEXT NOT NULL DEFAULT '',
    created_at        INTEGER NOT NULL,
    updated_at        INTEGER NOT NULL,
    UNIQUE (rule_set_id, priority),
    CHECK (CASE entry_type
        WHEN 'item' THEN applies_to_set_id IS NOT NULL AND action IS NOT NULL AND child_rule_set_id IS NULL
        WHEN 'set' THEN child_rule_set_id IS NOT NULL AND COALESCE(applies_to_set_id, action, match_type, pattern) IS NULL
    END)
) STRICT;
CREATE INDEX rule_entries_child ON rule_entries (child_rule_set_id);
CREATE INDEX rule_entries_applies_to ON rule_entries (applies_to_set_id);
CREATE UNIQUE INDEX rule_entries_one_fallback ON rule_entries (rule_set_id) WHERE is_fallback = 1;

-- The cookie carries an HMAC-signed session ID; only its hash is stored.
CREATE TABLE edge_sessions (
    id_hash      TEXT PRIMARY KEY,
    provider_id  TEXT NOT NULL REFERENCES oidc_providers (id) ON DELETE CASCADE,
    subject      TEXT NOT NULL,
    username     TEXT NOT NULL,
    email        TEXT NOT NULL,
    groups_json  TEXT NOT NULL,
    id_token     TEXT NOT NULL,
    ip           TEXT NOT NULL,
    user_agent   TEXT NOT NULL,
    created_at   INTEGER NOT NULL,
    last_seen_at INTEGER NOT NULL,
    expires_at   INTEGER NOT NULL
) STRICT;
CREATE INDEX edge_sessions_expires ON edge_sessions (expires_at);

-- +goose Down
DROP TABLE edge_sessions;
DROP TABLE rule_entries;
DROP TABLE oidc_providers;
DROP TABLE rule_sets;
