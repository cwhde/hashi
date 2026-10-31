-- name: ListFirewallHosts :many
SELECT * FROM firewall_hosts ORDER BY name;

-- name: GetFirewallHost :one
SELECT * FROM firewall_hosts WHERE id = ?;

-- name: InsertFirewallHost :exec
INSERT INTO firewall_hosts (id, name, edge_mode, ssh_host, ssh_port, ssh_user, auth, credential_secret_id,
    passphrase_secret_id, host_key_type, host_key, host_key_fingerprint, traefik_connection_id, internal_traefik_ip,
    route_via_host_id, public_ip_override, public_ip6_override, wan_iface_override, subnets_json, netbird_mode,
    netbird_iface, netbird_port, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateFirewallHostSettings :exec
UPDATE firewall_hosts
SET name = ?, traefik_connection_id = ?, internal_traefik_ip = ?, route_via_host_id = ?, public_ip_override = ?,
    public_ip6_override = ?, wan_iface_override = ?, subnets_json = ?, netbird_mode = ?, netbird_iface = ?,
    netbird_port = ?, drop_inbound_accepted_at = ?, updated_at = ?
WHERE id = ?;

-- name: SetFirewallHostDetected :exec
UPDATE firewall_hosts
SET public_ip = ?, public_ip6 = ?, wan_iface = ?, internal_ips_json = ?, facts_json = ?, health = ?,
    last_validated_at = ?, last_validation_error = ?, updated_at = ?
WHERE id = ?;

-- name: SetFirewallHostSetupRuleset :exec
UPDATE firewall_hosts SET setup_ruleset = ?, updated_at = ? WHERE id = ? AND setup_ruleset = '';

-- name: SetFirewallHostHealth :exec
UPDATE firewall_hosts SET health = ?, last_validation_error = ?, updated_at = ? WHERE id = ?;

-- name: ConfirmFirewallHostOwnership :exec
UPDATE firewall_hosts SET ownership_confirmed_at = ?, updated_at = ? WHERE id = ?;

-- name: SetFirewallHostApplied :exec
UPDATE firewall_hosts SET last_applied_hash = ?, last_applied_at = ?, updated_at = ? WHERE id = ?;

-- name: DeleteFirewallHost :exec
DELETE FROM firewall_hosts WHERE id = ?;

-- name: ListFirewallBindings :many
SELECT * FROM firewall_bindings ORDER BY position, ord;

-- name: DeleteFirewallBindings :exec
DELETE FROM firewall_bindings;

-- name: InsertFirewallBinding :exec
INSERT INTO firewall_bindings (position, ip_set_id, ord, created_at) VALUES (?, ?, ?, ?);

-- name: ListFirewallHostAccess :many
SELECT * FROM firewall_host_access WHERE host_id = ? ORDER BY created_at, ip_set_id;

-- name: ListAllFirewallHostAccess :many
SELECT * FROM firewall_host_access ORDER BY host_id, created_at;

-- name: DeleteFirewallHostAccess :exec
DELETE FROM firewall_host_access WHERE host_id = ?;

-- name: InsertFirewallHostAccess :exec
INSERT INTO firewall_host_access (host_id, ip_set_id, created_at) VALUES (?, ?, ?);

-- name: CountIPSetElementsForHost :one
SELECT COUNT(*) FROM ip_set_elements WHERE host_id = ?;

-- name: SetFirewallHostTakeover :exec
UPDATE firewall_hosts SET takeover_json = ?, updated_at = ? WHERE id = ?;
