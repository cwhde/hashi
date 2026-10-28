-- +goose Up
-- S5: HTTP-family services (04 §1) and their advanced routes. Every Traefik host receives every enabled, valid service
-- (04 §7), so services have no sync subsystem of their own: saving one plans the traefik subsystem of every host.

-- A service Hashi publishes. TCP/UDP kinds, address-set targets and rule sets arrive with S6/S9; until then the target is
-- a single literal address. host_name is the full public name; domain_mode only says how the user entered it.
CREATE TABLE services (
    id                   TEXT PRIMARY KEY,
    name                 TEXT NOT NULL,
    slug                 TEXT NOT NULL UNIQUE,
    kind                 TEXT NOT NULL CHECK (kind IN ('http', 'https', 'h2c')),
    domain_mode          TEXT NOT NULL CHECK (domain_mode IN ('root', 'subdomain', 'custom')),
    host_name            TEXT NOT NULL UNIQUE,
    enabled              INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    origin               TEXT NOT NULL CHECK (origin IN ('system', 'managed', 'imported', 'user')),
    -- The feature that owns a generated service ('setup' for the Hashi admin service); '' for user services.
    owner_feature        TEXT NOT NULL DEFAULT '',
    target_address       TEXT NOT NULL,
    target_port          INTEGER NOT NULL CHECK (target_port BETWEEN 1 AND 65535),
    -- kind 'https' only: the upstream's certificate check.
    tls_skip_verify      INTEGER NOT NULL DEFAULT 0 CHECK (tls_skip_verify IN (0, 1)),
    tls_server_name      TEXT NOT NULL DEFAULT '',
    -- Minimum TLS version served to clients: '' = Traefik's default.
    tls_min_version      TEXT NOT NULL DEFAULT '' CHECK (tls_min_version IN ('', 'VersionTLS12', 'VersionTLS13')),
    advanced_routes      INTEGER NOT NULL DEFAULT 0 CHECK (advanced_routes IN (0, 1)),
    dashboard_visible    INTEGER NOT NULL DEFAULT 1 CHECK (dashboard_visible IN (0, 1)),
    monitoring_visible   INTEGER NOT NULL DEFAULT 1 CHECK (monitoring_visible IN (0, 1)),
    adguard_rewrite      INTEGER NOT NULL DEFAULT 1 CHECK (adguard_rewrite IN (0, 1)),
    created_at           INTEGER NOT NULL,
    updated_at           INTEGER NOT NULL,
    CHECK (origin != 'system' OR owner_feature != ''),
    CHECK (kind = 'https' OR (tls_skip_verify = 0 AND tls_server_name = ''))
) STRICT;
CREATE UNIQUE INDEX services_owner ON services (owner_feature) WHERE owner_feature != '';

-- An advanced route (04 §1.1). Its identity is the ULID, never the priority; targets_json is
-- [{"address": "...", "port": n, "weight": n}] and load-balanced.
CREATE TABLE service_routes (
    id             TEXT PRIMARY KEY,
    service_id     TEXT NOT NULL REFERENCES services (id) ON DELETE CASCADE,
    enabled        INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    targets_json   TEXT NOT NULL,
    path_match     TEXT NOT NULL CHECK (path_match IN ('prefix', 'exact', 'regex')),
    path_value     TEXT NOT NULL,
    priority       INTEGER NOT NULL CHECK (priority BETWEEN 1 AND 9999),
    rewrite_mode   TEXT NOT NULL DEFAULT 'none'
                   CHECK (rewrite_mode IN ('none', 'replace_prefix', 'exact', 'regex', 'strip_prefix')),
    -- replace_prefix: the new prefix; exact: the new path; regex: the replacement (capture groups $1…).
    -- strip_prefix uses path_value itself. A regex rewrite matches against rewrite_pattern.
    rewrite_pattern TEXT NOT NULL DEFAULT '',
    rewrite_value  TEXT NOT NULL DEFAULT '',
    created_at     INTEGER NOT NULL,
    updated_at     INTEGER NOT NULL,
    UNIQUE (service_id, priority)
) STRICT;
CREATE INDEX service_routes_service ON service_routes (service_id);

-- +goose Down
DROP TABLE service_routes;
DROP TABLE services;
