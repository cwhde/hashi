-- name: ListRuleSets :many
SELECT * FROM rule_sets ORDER BY kind = 'global' DESC, slug;

-- name: GetRuleSet :one
SELECT * FROM rule_sets WHERE id = ?;

-- name: InsertRuleSet :exec
INSERT INTO rule_sets (id, slug, name, description, kind, origin, service_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateRuleSet :exec
UPDATE rule_sets SET slug = ?, name = ?, description = ?, updated_at = ? WHERE id = ?;

-- name: DeleteRuleSet :exec
DELETE FROM rule_sets WHERE id = ?;

-- name: ListRuleEntries :many
SELECT * FROM rule_entries ORDER BY rule_set_id, priority;

-- name: DeleteRuleEntries :exec
DELETE FROM rule_entries WHERE rule_set_id = ?;

-- name: InsertRuleEntry :exec
INSERT INTO rule_entries (id, rule_set_id, priority, entry_type, child_rule_set_id, applies_to_set_id, match_type, pattern,
    action, auth_provider_id, auth_groups_json, origin, locked, is_fallback, enabled, note, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListRuleItemsUsingIPSet :many
SELECT s.id, s.slug, s.name, s.kind, s.service_id FROM rule_entries e JOIN rule_sets s ON s.id = e.rule_set_id
WHERE e.applies_to_set_id = ? GROUP BY s.id ORDER BY s.slug;

-- name: ListOIDCProviders :many
SELECT * FROM oidc_providers ORDER BY is_default DESC, name;

-- name: GetOIDCProvider :one
SELECT * FROM oidc_providers WHERE id = ?;

-- name: InsertOIDCProvider :exec
INSERT INTO oidc_providers (id, name, issuer, client_id, secret_id, scopes, username_claim, email_claim, groups_claim,
    is_default, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateOIDCProvider :exec
UPDATE oidc_providers SET name = ?, issuer = ?, client_id = ?, secret_id = ?, scopes = ?, username_claim = ?, email_claim = ?,
    groups_claim = ?, is_default = ?, updated_at = ? WHERE id = ?;

-- name: ClearOIDCDefault :exec
UPDATE oidc_providers SET is_default = 0 WHERE is_default = 1;

-- name: DeleteOIDCProvider :exec
DELETE FROM oidc_providers WHERE id = ?;

-- name: CountRuleItemsUsingProvider :one
SELECT COUNT(*) FROM rule_entries WHERE auth_provider_id = ?;

-- name: InsertEdgeSession :exec
INSERT INTO edge_sessions (id_hash, provider_id, subject, username, email, groups_json, id_token, ip, user_agent, created_at,
    last_seen_at, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetEdgeSession :one
SELECT * FROM edge_sessions WHERE id_hash = ?;

-- name: TouchEdgeSession :exec
UPDATE edge_sessions SET last_seen_at = ? WHERE id_hash = ?;

-- name: DeleteEdgeSession :exec
DELETE FROM edge_sessions WHERE id_hash = ?;

-- name: DeleteExpiredEdgeSessions :exec
DELETE FROM edge_sessions WHERE expires_at <= ? OR last_seen_at <= ?;
