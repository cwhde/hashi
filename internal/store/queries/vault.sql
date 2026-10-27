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
