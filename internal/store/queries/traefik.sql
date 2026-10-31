-- name: UpsertACMEAccount :exec
INSERT INTO acme_accounts (id, directory, directory_url, email, eab_kid, eab_hmac_secret_id, account_key_secret_id,
    account_uri, dns_provider_id, propagation_delay_s, resolvers_json, health, last_validated_at, last_validation_error,
    created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
    directory = excluded.directory, directory_url = excluded.directory_url, email = excluded.email,
    eab_kid = excluded.eab_kid, eab_hmac_secret_id = excluded.eab_hmac_secret_id,
    account_key_secret_id = excluded.account_key_secret_id, account_uri = excluded.account_uri,
    dns_provider_id = excluded.dns_provider_id, propagation_delay_s = excluded.propagation_delay_s,
    resolvers_json = excluded.resolvers_json, health = excluded.health, last_validated_at = excluded.last_validated_at,
    last_validation_error = excluded.last_validation_error, updated_at = excluded.updated_at;

-- name: GetACMEAccount :one
-- Hashi has one ACME account (setup step 4).
SELECT * FROM acme_accounts ORDER BY created_at, id LIMIT 1;

-- name: InsertTraefikConnection :exec
INSERT INTO traefik_connections (id, name, ssh_host, ssh_port, ssh_user, auth, credential_secret_id, passphrase_secret_id,
    host_key_type, host_key, host_key_fingerprint, internal_ip, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateTraefikConnectionAccess :exec
-- Changing how Hashi logs in (or re-pinning a changed host key after the user confirmed the new fingerprint).
UPDATE traefik_connections
SET name = ?, ssh_host = ?, ssh_port = ?, ssh_user = ?, auth = ?, credential_secret_id = ?, passphrase_secret_id = ?,
    host_key_type = ?, host_key = ?, host_key_fingerprint = ?, internal_ip = ?, updated_at = ?
WHERE id = ?;

-- name: SetTraefikConnectionDetected :exec
UPDATE traefik_connections
SET os_family = ?, package_manager = ?, init_system = ?, os_probe = ?, discovery_json = ?, installed_version = ?,
    updated_at = ?
WHERE id = ?;

-- name: SetTraefikConnectionPaths :exec
UPDATE traefik_connections SET traefik_binary = ?, config_dir = ?, api_port = ?, updated_at = ? WHERE id = ?;

-- name: ConfirmTraefikOwnership :exec
UPDATE traefik_connections SET ownership_confirmed_at = ?, updated_at = ? WHERE id = ?;

-- name: SetTraefikConnectionHealth :exec
UPDATE traefik_connections SET health = ?, last_validated_at = ?, last_validation_error = ?, updated_at = ? WHERE id = ?;

-- name: SetTraefikInstalledVersion :exec
UPDATE traefik_connections SET installed_version = ?, updated_at = ? WHERE id = ?;

-- name: GetTraefikConnection :one
SELECT * FROM traefik_connections WHERE id = ?;

-- name: ListTraefikConnections :many
SELECT * FROM traefik_connections ORDER BY name;

-- name: DeleteTraefikConnection :exec
DELETE FROM traefik_connections WHERE id = ?;

-- name: ListTraefikFiles :many
SELECT * FROM traefik_files WHERE connection_id = ? ORDER BY path;

-- name: UpsertTraefikFile :exec
INSERT INTO traefik_files (connection_id, path, last_applied_hash, applied_at) VALUES (?, ?, ?, ?)
ON CONFLICT (connection_id, path) DO UPDATE SET last_applied_hash = excluded.last_applied_hash, applied_at = excluded.applied_at;

-- name: DeleteTraefikFile :exec
DELETE FROM traefik_files WHERE connection_id = ? AND path = ?;

-- name: InsertCertificate :exec
INSERT INTO certificates (id, name, names_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?);

-- name: SetCertificateNames :exec
-- A dropped wildcard leaves at the next renewal, never mid-term (03-dns.md 5.3): callers change names only when they renew.
UPDATE certificates SET names_json = ?, updated_at = ? WHERE id = ?;

-- name: SetCertificateIssued :exec
UPDATE certificates
SET names_json = ?, key_secret_id = ?, cert_pem = ?, issuer = ?, not_before = ?, not_after = ?, fingerprint = ?,
    last_renewal_at = ?, last_renewal_result = ?, last_renewal_error = '', updated_at = ?
WHERE id = ?;

-- name: SetCertificateRenewalFailed :exec
UPDATE certificates SET last_renewal_at = ?, last_renewal_result = 'failed', last_renewal_error = ?, updated_at = ? WHERE id = ?;

-- name: GetCertificate :one
SELECT * FROM certificates WHERE id = ?;

-- name: ListCertificates :many
SELECT * FROM certificates ORDER BY name;

-- name: UpsertCertificateDistribution :exec
INSERT INTO certificate_distributions (certificate_id, connection_id, fingerprint, state, last_verified_at, error, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (certificate_id, connection_id) DO UPDATE SET
    fingerprint = excluded.fingerprint, state = excluded.state, last_verified_at = excluded.last_verified_at,
    error = excluded.error, updated_at = excluded.updated_at;

-- name: ListCertificateDistributions :many
SELECT * FROM certificate_distributions ORDER BY certificate_id, connection_id;

-- name: SetTraefikConnectionSSHTarget :exec
UPDATE traefik_connections SET ssh_target_set_id = ? WHERE id = ?;

-- name: ListTraefikConnectionsBySSHTarget :many
SELECT id, name FROM traefik_connections WHERE ssh_target_set_id = ? ORDER BY name;
