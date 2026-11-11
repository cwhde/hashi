-- +goose Up
-- Vault mode (02 §5.4). No row = service mode.
CREATE TABLE vault_state (
    id         INTEGER PRIMARY KEY CHECK (id = 1),
    mode       TEXT NOT NULL CHECK (mode IN ('service', 'strict')),
    changed_at INTEGER NOT NULL
) STRICT;

-- Strict mode only: keys kept readable while the vault is locked, so automations keep running (owner, 2026-11-11):
-- the operational keys and the edge session cookie key. None of them opens a stored secret.
CREATE TABLE vault_kept (
    name  TEXT PRIMARY KEY,
    value BLOB NOT NULL
) STRICT;

-- Strict mode only: passkeys that unlock the vault. The master key is wrapped with a key derived from the passkey's PRF
-- output for salt; the PRF output itself is never stored.
CREATE TABLE vault_unlockers (
    id             TEXT PRIMARY KEY,
    passkey_id     TEXT NOT NULL UNIQUE REFERENCES passkey_credentials (id) ON DELETE CASCADE,
    salt           BLOB NOT NULL,
    key_version    INTEGER NOT NULL,
    nonce          BLOB NOT NULL,
    wrapped_master BLOB NOT NULL,
    created_at     INTEGER NOT NULL
) STRICT;

-- +goose Down
DROP TABLE vault_unlockers;
DROP TABLE vault_kept;
DROP TABLE vault_state;
