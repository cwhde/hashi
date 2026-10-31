-- +goose Up
-- S6: TCP/UDP stream services (04 §1.2). kind gains tcp and udp; a stream has a public port (one Traefik entry point
-- tcp-<port> / udp-<port>, DNAT on every managed firewall host) and, for TCP, the PROXY protocol option. SQLite cannot
-- change a CHECK in place: services and its child service_routes are rebuilt (the child first copied, so dropping the
-- parent cascades nothing).
CREATE TABLE services_v10 (
    id                   TEXT PRIMARY KEY,
    name                 TEXT NOT NULL,
    slug                 TEXT NOT NULL UNIQUE,
    kind                 TEXT NOT NULL CHECK (kind IN ('http', 'https', 'h2c', 'tcp', 'udp')),
    domain_mode          TEXT NOT NULL CHECK (domain_mode IN ('root', 'subdomain', 'custom')),
    host_name            TEXT NOT NULL UNIQUE,
    enabled              INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    origin               TEXT NOT NULL CHECK (origin IN ('system', 'managed', 'imported', 'user')),
    owner_feature        TEXT NOT NULL DEFAULT '',
    target_address       TEXT NOT NULL,
    target_port          INTEGER NOT NULL CHECK (target_port BETWEEN 1 AND 65535),
    tls_skip_verify      INTEGER NOT NULL DEFAULT 0 CHECK (tls_skip_verify IN (0, 1)),
    tls_server_name      TEXT NOT NULL DEFAULT '',
    tls_min_version      TEXT NOT NULL DEFAULT '' CHECK (tls_min_version IN ('', 'VersionTLS12', 'VersionTLS13')),
    advanced_routes      INTEGER NOT NULL DEFAULT 0 CHECK (advanced_routes IN (0, 1)),
    dashboard_visible    INTEGER NOT NULL DEFAULT 1 CHECK (dashboard_visible IN (0, 1)),
    monitoring_visible   INTEGER NOT NULL DEFAULT 1 CHECK (monitoring_visible IN (0, 1)),
    adguard_rewrite      INTEGER NOT NULL DEFAULT 1 CHECK (adguard_rewrite IN (0, 1)),
    created_at           INTEGER NOT NULL,
    updated_at           INTEGER NOT NULL,
    target_set_id        TEXT REFERENCES ip_sets (id) ON DELETE RESTRICT,
    entry_host_id        TEXT REFERENCES firewall_hosts (id) ON DELETE RESTRICT,
    -- Streams only: the public port (the entry point listens on the same port) and PROXY protocol v2 towards the
    -- upstream (TCP only). NULL / 0 for HTTP kinds.
    public_port          INTEGER CHECK (public_port BETWEEN 1 AND 65535),
    proxy_protocol       INTEGER NOT NULL DEFAULT 0 CHECK (proxy_protocol IN (0, 1)),
    CHECK (origin != 'system' OR owner_feature != ''),
    CHECK (kind = 'https' OR (tls_skip_verify = 0 AND tls_server_name = '')),
    CHECK ((kind IN ('tcp', 'udp')) = (public_port IS NOT NULL)),
    CHECK (kind = 'tcp' OR proxy_protocol = 0),
    CHECK (kind IN ('http', 'https', 'h2c') OR (advanced_routes = 0 AND tls_min_version = ''))
) STRICT;
INSERT INTO services_v10 (id, name, slug, kind, domain_mode, host_name, enabled, origin, owner_feature, target_address,
    target_port, tls_skip_verify, tls_server_name, tls_min_version, advanced_routes, dashboard_visible,
    monitoring_visible, adguard_rewrite, created_at, updated_at, target_set_id, entry_host_id)
SELECT id, name, slug, kind, domain_mode, host_name, enabled, origin, owner_feature, target_address, target_port,
    tls_skip_verify, tls_server_name, tls_min_version, advanced_routes, dashboard_visible, monitoring_visible,
    adguard_rewrite, created_at, updated_at, target_set_id, entry_host_id
FROM services;

CREATE TABLE service_routes_v10 (
    id             TEXT PRIMARY KEY,
    service_id     TEXT NOT NULL REFERENCES services_v10 (id) ON DELETE CASCADE,
    enabled        INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    targets_json   TEXT NOT NULL,
    path_match     TEXT NOT NULL CHECK (path_match IN ('prefix', 'exact', 'regex')),
    path_value     TEXT NOT NULL,
    priority       INTEGER NOT NULL CHECK (priority BETWEEN 1 AND 9999),
    rewrite_mode   TEXT NOT NULL DEFAULT 'none'
                   CHECK (rewrite_mode IN ('none', 'replace_prefix', 'exact', 'regex', 'strip_prefix')),
    rewrite_pattern TEXT NOT NULL DEFAULT '',
    rewrite_value  TEXT NOT NULL DEFAULT '',
    created_at     INTEGER NOT NULL,
    updated_at     INTEGER NOT NULL,
    UNIQUE (service_id, priority)
) STRICT;
INSERT INTO service_routes_v10 SELECT * FROM service_routes;

DROP TABLE service_routes;
DROP TABLE services;
-- Renaming the parent rewrites the child's REFERENCES to the new name.
ALTER TABLE services_v10 RENAME TO services;
ALTER TABLE service_routes_v10 RENAME TO service_routes;
CREATE UNIQUE INDEX services_owner ON services (owner_feature) WHERE owner_feature != '';
CREATE INDEX service_routes_service ON service_routes (service_id);
-- One stream per protocol and public port.
CREATE UNIQUE INDEX services_stream_port ON services (kind, public_port) WHERE public_port IS NOT NULL;

-- +goose Down
DELETE FROM services WHERE kind IN ('tcp', 'udp');
CREATE TABLE services_v9 (
    id                   TEXT PRIMARY KEY,
    name                 TEXT NOT NULL,
    slug                 TEXT NOT NULL UNIQUE,
    kind                 TEXT NOT NULL CHECK (kind IN ('http', 'https', 'h2c')),
    domain_mode          TEXT NOT NULL CHECK (domain_mode IN ('root', 'subdomain', 'custom')),
    host_name            TEXT NOT NULL UNIQUE,
    enabled              INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    origin               TEXT NOT NULL CHECK (origin IN ('system', 'managed', 'imported', 'user')),
    owner_feature        TEXT NOT NULL DEFAULT '',
    target_address       TEXT NOT NULL,
    target_port          INTEGER NOT NULL CHECK (target_port BETWEEN 1 AND 65535),
    tls_skip_verify      INTEGER NOT NULL DEFAULT 0 CHECK (tls_skip_verify IN (0, 1)),
    tls_server_name      TEXT NOT NULL DEFAULT '',
    tls_min_version      TEXT NOT NULL DEFAULT '' CHECK (tls_min_version IN ('', 'VersionTLS12', 'VersionTLS13')),
    advanced_routes      INTEGER NOT NULL DEFAULT 0 CHECK (advanced_routes IN (0, 1)),
    dashboard_visible    INTEGER NOT NULL DEFAULT 1 CHECK (dashboard_visible IN (0, 1)),
    monitoring_visible   INTEGER NOT NULL DEFAULT 1 CHECK (monitoring_visible IN (0, 1)),
    adguard_rewrite      INTEGER NOT NULL DEFAULT 1 CHECK (adguard_rewrite IN (0, 1)),
    created_at           INTEGER NOT NULL,
    updated_at           INTEGER NOT NULL,
    target_set_id        TEXT REFERENCES ip_sets (id) ON DELETE RESTRICT,
    entry_host_id        TEXT REFERENCES firewall_hosts (id) ON DELETE RESTRICT,
    CHECK (origin != 'system' OR owner_feature != ''),
    CHECK (kind = 'https' OR (tls_skip_verify = 0 AND tls_server_name = ''))
) STRICT;
INSERT INTO services_v9 (id, name, slug, kind, domain_mode, host_name, enabled, origin, owner_feature, target_address, target_port, tls_skip_verify, tls_server_name, tls_min_version, advanced_routes, dashboard_visible, monitoring_visible, adguard_rewrite, created_at, updated_at, target_set_id, entry_host_id) SELECT id, name, slug, kind, domain_mode, host_name, enabled, origin, owner_feature, target_address, target_port, tls_skip_verify, tls_server_name, tls_min_version, advanced_routes, dashboard_visible, monitoring_visible, adguard_rewrite, created_at, updated_at, target_set_id, entry_host_id FROM services;
CREATE TABLE service_routes_v9 (
    id             TEXT PRIMARY KEY,
    service_id     TEXT NOT NULL REFERENCES services_v9 (id) ON DELETE CASCADE,
    enabled        INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    targets_json   TEXT NOT NULL,
    path_match     TEXT NOT NULL CHECK (path_match IN ('prefix', 'exact', 'regex')),
    path_value     TEXT NOT NULL,
    priority       INTEGER NOT NULL CHECK (priority BETWEEN 1 AND 9999),
    rewrite_mode   TEXT NOT NULL DEFAULT 'none'
                   CHECK (rewrite_mode IN ('none', 'replace_prefix', 'exact', 'regex', 'strip_prefix')),
    rewrite_pattern TEXT NOT NULL DEFAULT '',
    rewrite_value  TEXT NOT NULL DEFAULT '',
    created_at     INTEGER NOT NULL,
    updated_at     INTEGER NOT NULL,
    UNIQUE (service_id, priority)
) STRICT;
INSERT INTO service_routes_v9 SELECT * FROM service_routes;
DROP TABLE service_routes;
DROP TABLE services;
ALTER TABLE services_v9 RENAME TO services;
ALTER TABLE service_routes_v9 RENAME TO service_routes;
CREATE UNIQUE INDEX services_owner ON services (owner_feature) WHERE owner_feature != '';
CREATE INDEX service_routes_service ON service_routes (service_id);
