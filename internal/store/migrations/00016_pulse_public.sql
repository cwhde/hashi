-- +goose Up
-- An agent may report its own public address when it cannot reach Hashi over a public path. Off by default: the
-- address can feed firewall rules, so trusting it is the admin's choice per agent (08 §4).
ALTER TABLE pulse_agents ADD COLUMN trust_reported_ip INTEGER NOT NULL DEFAULT 0 CHECK (trust_reported_ip IN (0, 1));
ALTER TABLE pulse_agents ADD COLUMN public_ip_source TEXT NOT NULL DEFAULT '' CHECK (public_ip_source IN ('', 'connection', 'reported'));

-- +goose Down
ALTER TABLE pulse_agents DROP COLUMN public_ip_source;
ALTER TABLE pulse_agents DROP COLUMN trust_reported_ip;
