-- name: InsertSecret :exec
INSERT INTO secret_records (id, class, purpose, key_version, nonce, ciphertext, dek_nonce, wrapped_dek, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetSecret :one
SELECT * FROM secret_records WHERE id = ?;

-- name: GetSecretByPurpose :one
SELECT * FROM secret_records WHERE purpose = ? ORDER BY created_at LIMIT 1;

-- name: ListSecretMeta :many
SELECT id, class, purpose, key_version, created_at, updated_at FROM secret_records ORDER BY purpose, id;

-- name: UpsertRecoveryKey :exec
INSERT INTO recovery_key (id, key_version, verifier, nonce, wrapped_master, created_at)
VALUES (1, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET key_version = excluded.key_version, verifier = excluded.verifier,
    nonce = excluded.nonce, wrapped_master = excluded.wrapped_master, created_at = excluded.created_at, confirmed_at = NULL;

-- name: GetRecoveryKey :one
SELECT * FROM recovery_key WHERE id = 1;

-- name: ConfirmRecoveryKey :exec
UPDATE recovery_key SET confirmed_at = ? WHERE id = 1;

-- name: DeleteSecret :exec
DELETE FROM secret_records WHERE id = ?;

-- name: DeleteSecretsByPurpose :exec
DELETE FROM secret_records WHERE purpose = ?;

-- name: ListSecrets :many
SELECT * FROM secret_records ORDER BY id;

-- name: ResealSecret :exec
UPDATE secret_records SET key_version = ?, nonce = ?, ciphertext = ?, dek_nonce = ?, wrapped_dek = ?, updated_at = ?
WHERE id = ?;

-- name: RewrapRecoveryKey :exec
UPDATE recovery_key SET key_version = ?, nonce = ?, wrapped_master = ? WHERE id = 1;

-- name: GetVaultMode :one
SELECT mode FROM vault_state WHERE id = 1;

-- name: SetVaultMode :exec
INSERT INTO vault_state (id, mode, changed_at) VALUES (1, ?, ?)
ON CONFLICT (id) DO UPDATE SET mode = excluded.mode, changed_at = excluded.changed_at;

-- name: ListVaultKept :many
SELECT * FROM vault_kept ORDER BY name;

-- name: PutVaultKept :exec
INSERT INTO vault_kept (name, value) VALUES (?, ?) ON CONFLICT (name) DO UPDATE SET value = excluded.value;

-- name: DeleteVaultKept :exec
DELETE FROM vault_kept;

-- name: InsertVaultUnlocker :exec
INSERT INTO vault_unlockers (id, passkey_id, salt, key_version, nonce, wrapped_master, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (passkey_id) DO UPDATE SET id = excluded.id, salt = excluded.salt, key_version = excluded.key_version,
    nonce = excluded.nonce, wrapped_master = excluded.wrapped_master, created_at = excluded.created_at;

-- name: ListVaultUnlockers :many
SELECT u.*, p.name AS passkey_name, p.credential_id
FROM vault_unlockers u JOIN passkey_credentials p ON p.id = u.passkey_id ORDER BY u.created_at;

-- name: DeleteVaultUnlocker :execrows
DELETE FROM vault_unlockers WHERE id = ?;

-- name: DeleteVaultUnlockers :exec
DELETE FROM vault_unlockers;
