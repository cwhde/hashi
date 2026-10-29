-- +goose Up
-- S6: IP sets, the one address primitive (05a, 06a §2.1–2.2). Every consumer that needs an address references a set.

-- A named set. kind decides what the user may edit (05a §3); system sets carry a system_key and are seeded below.
-- The compiled_* and resolution_* columns cache the last compile for lists and the address page; the compiler is the
-- only writer.
CREATE TABLE ip_sets (
    id                       TEXT PRIMARY KEY,
    slug                     TEXT NOT NULL UNIQUE,
    name                     TEXT NOT NULL,
    description              TEXT NOT NULL DEFAULT '',
    kind                     TEXT NOT NULL CHECK (kind IN ('user', 'system', 'feed', 'dynamic')),
    origin                   TEXT NOT NULL CHECK (origin IN ('system', 'managed', 'imported', 'user')),
    system_key               TEXT CHECK (system_key IN ('all', 'hashi_hosts', 'private_networks',
                                                        'dynamic_challenge', 'dynamic_blocked')),
    compiled_hash            TEXT NOT NULL DEFAULT '',
    resolution_status        TEXT NOT NULL DEFAULT 'empty' CHECK (resolution_status IN ('ok', 'partial', 'empty')),
    resolution_problems_json TEXT NOT NULL DEFAULT '[]',
    compiled_at              INTEGER,
    created_at               INTEGER NOT NULL,
    updated_at               INTEGER NOT NULL,
    CHECK ((kind IN ('system', 'dynamic')) = (system_key IS NOT NULL))
) STRICT;
CREATE UNIQUE INDEX ip_sets_system_key ON ip_sets (system_key) WHERE system_key IS NOT NULL;

-- One element: an IP item or a reference to another set. Exactly the columns of its type are set.
-- host_id and pulse_agent_id carry no foreign key: their tables arrive later (firewall hosts in this slice's second
-- migration, Pulse agents in S8); deleting a referenced host or agent is refused by its owner (05a §2).
-- child_set_id is RESTRICT: a referenced set cannot be deleted (05a §2).
CREATE TABLE ip_set_elements (
    id             TEXT PRIMARY KEY,
    ip_set_id      TEXT NOT NULL REFERENCES ip_sets (id) ON DELETE CASCADE,
    position       INTEGER NOT NULL,
    type           TEXT NOT NULL CHECK (type IN ('ip', 'cidr', 'pulse', 'host', 'dns', 'geo', 'set')),
    origin         TEXT NOT NULL CHECK (origin IN ('system', 'managed', 'imported', 'user', 'feed', 'engine')),
    locked         INTEGER NOT NULL DEFAULT 0 CHECK (locked IN (0, 1)),
    address        TEXT,
    prefix         TEXT,
    family         INTEGER CHECK (family IN (4, 6)),
    pulse_agent_id TEXT,
    host_id        TEXT,
    scope          TEXT CHECK (scope IN ('public', 'internal', 'all')),
    selector       TEXT,
    fqdn           TEXT,
    geo_kind       TEXT CHECK (geo_kind IN ('country', 'region', 'asn')),
    geo_value      TEXT,
    child_set_id   TEXT REFERENCES ip_sets (id) ON DELETE RESTRICT,
    note           TEXT NOT NULL DEFAULT '',
    expires_at     INTEGER,
    created_by     TEXT NOT NULL DEFAULT '',
    created_at     INTEGER NOT NULL,
    updated_at     INTEGER NOT NULL,
    UNIQUE (ip_set_id, position),
    CHECK (child_set_id IS NULL OR child_set_id != ip_set_id),
    CHECK (CASE type
        WHEN 'ip' THEN address IS NOT NULL AND family IS NOT NULL AND COALESCE(prefix, pulse_agent_id, host_id, scope,
            selector, fqdn, geo_kind, geo_value, child_set_id) IS NULL
        WHEN 'cidr' THEN prefix IS NOT NULL AND family IS NOT NULL AND COALESCE(address, pulse_agent_id, host_id, scope,
            selector, fqdn, geo_kind, geo_value, child_set_id) IS NULL
        WHEN 'pulse' THEN pulse_agent_id IS NOT NULL AND scope IS NOT NULL AND COALESCE(address, prefix, family,
            host_id, fqdn, geo_kind, geo_value, child_set_id) IS NULL
        WHEN 'host' THEN host_id IS NOT NULL AND scope IS NOT NULL AND COALESCE(address, prefix, family,
            pulse_agent_id, selector, fqdn, geo_kind, geo_value, child_set_id) IS NULL
        WHEN 'dns' THEN fqdn IS NOT NULL AND COALESCE(address, prefix, family, pulse_agent_id, host_id, scope,
            selector, geo_kind, geo_value, child_set_id) IS NULL
        WHEN 'geo' THEN geo_kind IS NOT NULL AND geo_value IS NOT NULL AND COALESCE(address, prefix, family,
            pulse_agent_id, host_id, scope, selector, fqdn, child_set_id) IS NULL
        WHEN 'set' THEN child_set_id IS NOT NULL AND COALESCE(address, prefix, family, pulse_agent_id, host_id,
            scope, selector, fqdn, geo_kind, geo_value) IS NULL
    END)
) STRICT;
CREATE INDEX ip_set_elements_address ON ip_set_elements (ip_set_id, address);
CREATE INDEX ip_set_elements_prefix ON ip_set_elements (ip_set_id, prefix);
CREATE INDEX ip_set_elements_child ON ip_set_elements (child_set_id);
CREATE INDEX ip_set_elements_pulse ON ip_set_elements (pulse_agent_id);
CREATE INDEX ip_set_elements_host ON ip_set_elements (host_id);
CREATE INDEX ip_set_elements_expires ON ip_set_elements (expires_at);

-- System sets (05a §3). hashi-hosts gets one host element per registered firewall/edge host from the firewall feature.
INSERT INTO ip_sets (id, slug, name, description, kind, origin, system_key, created_at, updated_at) VALUES
    ('sys-all', 'all', 'all', 'Every address (IPv4 and IPv6).', 'system', 'system', 'all',
        unixepoch() * 1000, unixepoch() * 1000),
    ('sys-hashi-hosts', 'hashi-hosts', 'hashi-hosts', 'The public addresses of every registered firewall and edge host.',
        'system', 'system', 'hashi_hosts', unixepoch() * 1000, unixepoch() * 1000),
    ('sys-private-networks', 'private-networks', 'private-networks',
        'Private, shared, loopback, link-local and unique-local ranges.', 'system', 'system', 'private_networks',
        unixepoch() * 1000, unixepoch() * 1000);

INSERT INTO ip_set_elements (id, ip_set_id, position, type, origin, locked, prefix, address, family, note, created_by,
    created_at, updated_at) VALUES
    ('sys-all-4', 'sys-all', 1, 'cidr', 'system', 1, '0.0.0.0/0', NULL, 4, '', 'system', unixepoch() * 1000, unixepoch() * 1000),
    ('sys-all-6', 'sys-all', 2, 'cidr', 'system', 1, '::/0', NULL, 6, '', 'system', unixepoch() * 1000, unixepoch() * 1000),
    ('sys-pn-1', 'sys-private-networks', 1, 'cidr', 'system', 1, '10.0.0.0/8', NULL, 4, 'RFC 1918', 'system',
        unixepoch() * 1000, unixepoch() * 1000),
    ('sys-pn-2', 'sys-private-networks', 2, 'cidr', 'system', 1, '172.16.0.0/12', NULL, 4, 'RFC 1918', 'system',
        unixepoch() * 1000, unixepoch() * 1000),
    ('sys-pn-3', 'sys-private-networks', 3, 'cidr', 'system', 1, '192.168.0.0/16', NULL, 4, 'RFC 1918', 'system',
        unixepoch() * 1000, unixepoch() * 1000),
    ('sys-pn-4', 'sys-private-networks', 4, 'cidr', 'system', 1, '100.64.0.0/10', NULL, 4, 'RFC 6598 shared address space',
        'system', unixepoch() * 1000, unixepoch() * 1000),
    ('sys-pn-5', 'sys-private-networks', 5, 'cidr', 'system', 1, '127.0.0.0/8', NULL, 4, 'loopback', 'system',
        unixepoch() * 1000, unixepoch() * 1000),
    ('sys-pn-6', 'sys-private-networks', 6, 'cidr', 'system', 1, '169.254.0.0/16', NULL, 4, 'link-local', 'system',
        unixepoch() * 1000, unixepoch() * 1000),
    ('sys-pn-7', 'sys-private-networks', 7, 'ip', 'system', 1, NULL, '::1', 6, 'loopback', 'system',
        unixepoch() * 1000, unixepoch() * 1000),
    ('sys-pn-8', 'sys-private-networks', 8, 'cidr', 'system', 1, 'fe80::/10', NULL, 6, 'link-local', 'system',
        unixepoch() * 1000, unixepoch() * 1000),
    ('sys-pn-9', 'sys-private-networks', 9, 'cidr', 'system', 1, 'fc00::/7', NULL, 6, 'unique local (ULA)', 'system',
        unixepoch() * 1000, unixepoch() * 1000);

-- +goose Down
DROP TABLE ip_set_elements;
DROP TABLE ip_sets;
