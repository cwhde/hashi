-- name: GetPulseConfig :one
SELECT * FROM pulse_config WHERE id = 1;

-- name: InsertPulseConfig :exec
INSERT INTO pulse_config (id, pepper_secret_id) VALUES (1, ?);

-- name: ListPulseAgents :many
SELECT * FROM pulse_agents ORDER BY name;

-- name: GetPulseAgent :one
SELECT * FROM pulse_agents WHERE id = ?;

-- name: InsertPulseAgent :exec
INSERT INTO pulse_agents (id, name, token_hash, install_type, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?);

-- name: SetPulseToken :exec
UPDATE pulse_agents SET token_hash = ?, revoked_at = NULL, updated_at = ? WHERE id = ?;

-- name: RevokePulseAgent :exec
UPDATE pulse_agents SET revoked_at = ?, updated_at = ? WHERE id = ?;

-- name: SetPulseHeartbeat :exec
UPDATE pulse_agents SET last_seen_at = ?, public_ip = ?, candidates_json = ?, selected_ip = ?, hostname = ?, version = ?, updated_at = ? WHERE id = ?;

-- name: SetPulseDNS :exec
UPDATE pulse_agents SET name = ?, dns_enabled = ?, dns_name = ?, updated_at = ? WHERE id = ?;

-- name: DeletePulseAgent :exec
DELETE FROM pulse_agents WHERE id = ?;

-- name: ListIPSetsUsingAgent :many
SELECT DISTINCT s.name FROM ip_set_elements e JOIN ip_sets s ON s.id = e.ip_set_id WHERE e.pulse_agent_id = ? ORDER BY s.name;
