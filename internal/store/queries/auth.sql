-- name: GetAdminUser :one
SELECT * FROM admin_users WHERE id = 1;

-- name: InsertAdminUser :exec
INSERT INTO admin_users (id, user_handle, created_at) VALUES (1, ?, ?) ON CONFLICT (id) DO NOTHING;

-- name: GetBootstrapCredentials :one
SELECT * FROM bootstrap_credentials WHERE id = 1;

-- name: UpsertBootstrapCredentials :exec
INSERT INTO bootstrap_credentials (id, username, password_hash, created_at) VALUES (1, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET username = excluded.username, password_hash = excluded.password_hash,
    created_at = excluded.created_at;

-- name: DeleteBootstrapCredentials :exec
DELETE FROM bootstrap_credentials WHERE id = 1;

-- name: InsertPasskey :exec
INSERT INTO passkey_credentials (id, credential_id, credential_json, name, aaguid_label, created_at)
VALUES (?, ?, ?, ?, ?, ?);

-- name: ListPasskeys :many
SELECT * FROM passkey_credentials ORDER BY created_at, id;

-- name: GetPasskey :one
SELECT * FROM passkey_credentials WHERE id = ?;

-- name: GetPasskeyByCredentialID :one
SELECT * FROM passkey_credentials WHERE credential_id = ?;

-- name: CountPasskeys :one
SELECT COUNT(*) FROM passkey_credentials;

-- name: UpdatePasskeyUse :exec
UPDATE passkey_credentials SET credential_json = ?, last_used_at = ? WHERE id = ?;

-- name: RenamePasskey :execrows
UPDATE passkey_credentials SET name = ? WHERE id = ?;

-- name: DeletePasskey :execrows
DELETE FROM passkey_credentials WHERE id = ?;

-- name: InsertSession :exec
INSERT INTO admin_sessions (id_hash, auth_method, credential_id, bound_ip, scopes_json, created_at, last_seen_at,
    idle_expires_at, absolute_expires_at, user_agent_hash)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetSession :one
SELECT * FROM admin_sessions WHERE id_hash = ?;

-- name: TouchSession :exec
UPDATE admin_sessions SET last_seen_at = ?, idle_expires_at = ? WHERE id_hash = ? AND revoked_at IS NULL;

-- name: SetSessionReauth :exec
UPDATE admin_sessions SET reauth_at = ? WHERE id_hash = ? AND revoked_at IS NULL;

-- name: RevokeSession :execrows
UPDATE admin_sessions SET revoked_at = ?, revocation_reason = ? WHERE id_hash = ? AND revoked_at IS NULL;

-- name: RevokeSessionsByMethod :execrows
UPDATE admin_sessions SET revoked_at = ?, revocation_reason = ? WHERE auth_method = ? AND revoked_at IS NULL;

-- name: RevokeSessionsByCredential :execrows
UPDATE admin_sessions SET revoked_at = ?, revocation_reason = ? WHERE credential_id = ? AND revoked_at IS NULL;

-- name: RevokeOtherSessions :execrows
UPDATE admin_sessions SET revoked_at = ?, revocation_reason = ? WHERE id_hash <> ? AND revoked_at IS NULL;

-- name: ListActiveSessions :many
SELECT * FROM admin_sessions
WHERE revoked_at IS NULL AND idle_expires_at > sqlc.arg(now) AND absolute_expires_at > sqlc.arg(now)
ORDER BY created_at DESC;

-- Expired sessions, and revoked ones older than the audit-retention cutoff (spec 02 section 3.6).
-- name: DeleteDeadSessions :execrows
DELETE FROM admin_sessions
WHERE (revoked_at IS NULL AND MIN(absolute_expires_at, idle_expires_at) <= sqlc.arg(now))
   OR revoked_at <= CAST(sqlc.arg(revoked_before) AS INTEGER);
