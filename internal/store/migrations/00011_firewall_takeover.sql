-- +goose Up
-- S6: what Hashi removes from a host's old firewall script on apply (05 3.4): explicit chains, rules, ipsets, cron lines.
ALTER TABLE firewall_hosts ADD COLUMN takeover_json TEXT NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE firewall_hosts DROP COLUMN takeover_json;
