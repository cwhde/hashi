-- name: InsertRecoveryLink :exec
INSERT INTO recovery_links (token_hash, created_at, expires_at) VALUES (?, ?, ?);

-- name: UseRecoveryLink :execrows
UPDATE recovery_links SET used_at = CAST(sqlc.arg(now) AS INTEGER)
WHERE token_hash = sqlc.arg(token_hash) AND used_at IS NULL AND expires_at > CAST(sqlc.arg(now) AS INTEGER);
