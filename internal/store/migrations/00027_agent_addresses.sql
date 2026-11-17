-- +goose Up
-- 0.13.3: the admin's choice of an agent's main address (bug 22; empty = the agent's own choice), and a firewall host
-- whose public and internal addresses follow a Pulse agent (bug 23, 05 §1.1 external_nat).
ALTER TABLE pulse_agents ADD COLUMN pinned_ip TEXT NOT NULL DEFAULT '';
ALTER TABLE firewall_hosts ADD COLUMN address_agent_id TEXT REFERENCES pulse_agents (id) ON DELETE RESTRICT;

-- +goose Down
ALTER TABLE firewall_hosts DROP COLUMN address_agent_id;
ALTER TABLE pulse_agents DROP COLUMN pinned_ip;
