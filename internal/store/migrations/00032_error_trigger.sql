-- +goose Up
-- 0.13.6, from a measurement of real traffic (06 sections 3.2 and 10, bug 26).
-- A challenge can now come from the error trigger: too many 4xx answers in a short window.
CREATE TABLE dynamic_entry_state_new (
    element_id       TEXT PRIMARY KEY REFERENCES ip_set_elements (id) ON DELETE CASCADE,
    source           TEXT NOT NULL CHECK (source IN ('l7_rate', 'l7_waf', 'l7_errors', 'l7_challenge_failed', 'l4_meter', 'manual')),
    meter            TEXT NOT NULL DEFAULT '',
    firewall_host_id TEXT REFERENCES firewall_hosts (id) ON DELETE SET NULL,
    service_id       TEXT REFERENCES services (id) ON DELETE SET NULL,
    reason           TEXT NOT NULL DEFAULT '',
    failure_count    INTEGER NOT NULL DEFAULT 0,
    removed_at       INTEGER,
    removed_reason   TEXT NOT NULL DEFAULT ''
) STRICT;
INSERT INTO dynamic_entry_state_new (element_id, source, meter, firewall_host_id, service_id, reason, failure_count, removed_at, removed_reason) SELECT element_id, source, meter, firewall_host_id, service_id, reason, failure_count, removed_at, removed_reason FROM dynamic_entry_state;
DROP TABLE dynamic_entry_state;
ALTER TABLE dynamic_entry_state_new RENAME TO dynamic_entry_state;

-- An install still on the old request limit default (300 in 10 s) moves to 1600; a value the owner chose stays. The
-- error trigger's settings need no row change: missing fields load as their defaults.
UPDATE app_settings SET value_json = json_set(value_json, '$.rate_requests', 1600)
WHERE key = 'engine' AND json_extract(value_json, '$.rate_requests') = 300
  AND json_extract(value_json, '$.rate_window_seconds') = 10;

-- +goose Down
UPDATE app_settings SET value_json = json_set(value_json, '$.rate_requests', 300)
WHERE key = 'engine' AND json_extract(value_json, '$.rate_requests') = 1600
  AND json_extract(value_json, '$.rate_window_seconds') = 10;
CREATE TABLE dynamic_entry_state_old (
    element_id       TEXT PRIMARY KEY REFERENCES ip_set_elements (id) ON DELETE CASCADE,
    source           TEXT NOT NULL CHECK (source IN ('l7_rate', 'l7_waf', 'l7_challenge_failed', 'l4_meter', 'manual')),
    meter            TEXT NOT NULL DEFAULT '',
    firewall_host_id TEXT REFERENCES firewall_hosts (id) ON DELETE SET NULL,
    service_id       TEXT REFERENCES services (id) ON DELETE SET NULL,
    reason           TEXT NOT NULL DEFAULT '',
    failure_count    INTEGER NOT NULL DEFAULT 0,
    removed_at       INTEGER,
    removed_reason   TEXT NOT NULL DEFAULT ''
) STRICT;
INSERT INTO dynamic_entry_state_old (element_id, source, meter, firewall_host_id, service_id, reason, failure_count, removed_at, removed_reason)
SELECT element_id, CASE source WHEN 'l7_errors' THEN 'l7_rate' ELSE source END, meter, firewall_host_id, service_id,
       reason, failure_count, removed_at, removed_reason FROM dynamic_entry_state;
DROP TABLE dynamic_entry_state;
ALTER TABLE dynamic_entry_state_old RENAME TO dynamic_entry_state;
