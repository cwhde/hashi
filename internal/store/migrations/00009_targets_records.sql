-- +goose Up
-- S6: address sets in target positions (05a §5.2) and generated DNS records (03 §5.1–5.2).

-- A service's simple target is either a literal address (target_address) or an address set (target_set_id); route
-- targets carry "set_id" in targets_json the same way. entry_host_id is the service routing override (03 §5.1).
-- Deleting a set or host that a service uses is refused by its owner (consumer list); RESTRICT is the backstop.
ALTER TABLE services ADD COLUMN target_set_id TEXT REFERENCES ip_sets (id) ON DELETE RESTRICT;
ALTER TABLE services ADD COLUMN entry_host_id TEXT REFERENCES firewall_hosts (id) ON DELETE RESTRICT;

-- An A/AAAA entry whose value is an address set (03 §7): values_json then holds nothing of its own.
ALTER TABLE dns_records ADD COLUMN value_set_id TEXT REFERENCES ip_sets (id) ON DELETE RESTRICT;

-- The last good resolution of every target position (invariant 21): a consumer whose set resolves to nothing uses
-- these addresses and shows unresolved_reason. consumer is "<type>:<id>[:<part>]" (e.g. "service:01H…:route:01H…").
CREATE TABLE ip_set_targets (
    consumer          TEXT NOT NULL,
    ip_set_id         TEXT NOT NULL REFERENCES ip_sets (id) ON DELETE CASCADE,
    addresses_json    TEXT NOT NULL DEFAULT '[]',
    unresolved_reason TEXT NOT NULL DEFAULT '',
    resolved_at       INTEGER,
    updated_at        INTEGER NOT NULL,
    PRIMARY KEY (consumer, ip_set_id)
) STRICT;

-- +goose Down
DROP TABLE ip_set_targets;
ALTER TABLE dns_records DROP COLUMN value_set_id;
ALTER TABLE services DROP COLUMN entry_host_id;
ALTER TABLE services DROP COLUMN target_set_id;
