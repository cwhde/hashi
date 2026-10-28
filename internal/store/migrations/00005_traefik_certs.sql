-- +goose Up
-- S4: Hashi's ACME account (02 §1.4), Traefik connections (04 §5), applied files (04 §6), Hashi-issued certificates and
-- their per-host distribution (03 §5.3), and the sync subsystems traefik and certificates.

-- The ACME account Hashi issues certificates with (setup step 4). One row: Hashi manages one zone.
-- EAB HMAC and account key live only in the vault. account_uri is set once the CA registered the account.
CREATE TABLE acme_accounts (
    id                     TEXT PRIMARY KEY,
    directory              TEXT NOT NULL CHECK (directory IN ('gts', 'letsencrypt', 'custom')),
    directory_url          TEXT NOT NULL,
    email                  TEXT NOT NULL,
    eab_kid                TEXT NOT NULL DEFAULT '',
    eab_hmac_secret_id     TEXT REFERENCES secret_records (id),
    account_key_secret_id  TEXT REFERENCES secret_records (id),
    account_uri            TEXT NOT NULL DEFAULT '',
    dns_provider_id        TEXT NOT NULL REFERENCES dns_providers (id),
    -- Seconds to wait after writing _acme-challenge before the CA is asked to check it.
    propagation_delay_s    INTEGER NOT NULL DEFAULT 30 CHECK (propagation_delay_s BETWEEN 0 AND 600),
    -- Resolvers used to see the TXT value before asking the CA; [] = the provider's authoritative servers.
    resolvers_json         TEXT NOT NULL DEFAULT '[]',
    health                 TEXT NOT NULL DEFAULT 'unknown'
                           CHECK (health IN ('unknown', 'validating', 'healthy', 'degraded', 'failed')),
    last_validated_at      INTEGER,
    last_validation_error  TEXT NOT NULL DEFAULT '',
    created_at             INTEGER NOT NULL,
    updated_at             INTEGER NOT NULL,
    -- GTS needs External Account Binding.
    CHECK (directory != 'gts' OR (eab_kid != '' AND eab_hmac_secret_id IS NOT NULL))
) STRICT;

-- A Traefik host Hashi manages over SSH (setup step 5). Credentials live only in the vault; auth 'hashi_key' uses
-- Hashi's own SSH key (public half shown to the user as an authorized_keys line).
-- The host key is pinned before anything else happens: no row exists without one (04 §5, 02 §1.5).
CREATE TABLE traefik_connections (
    id                     TEXT PRIMARY KEY,
    name                   TEXT NOT NULL UNIQUE,
    ssh_host               TEXT NOT NULL,
    ssh_port               INTEGER NOT NULL DEFAULT 22 CHECK (ssh_port BETWEEN 1 AND 65535),
    ssh_user               TEXT NOT NULL,
    auth                   TEXT NOT NULL CHECK (auth IN ('hashi_key', 'private_key', 'password')),
    credential_secret_id   TEXT REFERENCES secret_records (id),
    passphrase_secret_id   TEXT REFERENCES secret_records (id),
    host_key_type          TEXT NOT NULL,
    host_key               BLOB NOT NULL,
    host_key_fingerprint   TEXT NOT NULL,
    -- Used for DNAT and AdGuard rewrites (later slices).
    internal_ip            TEXT NOT NULL DEFAULT '',
    -- Detected from the OS probe (testdata/contracts/os): family, package manager, init system; never a version.
    os_family              TEXT NOT NULL DEFAULT '' CHECK (os_family IN ('', 'alpine', 'debian', 'fedora')),
    package_manager        TEXT NOT NULL DEFAULT '' CHECK (package_manager IN ('', 'apk', 'apt-get', 'dnf')),
    init_system            TEXT NOT NULL DEFAULT '' CHECK (init_system IN ('', 'openrc', 'systemd')),
    os_probe               TEXT NOT NULL DEFAULT '',
    -- Paths (04 §2–3 defaults, overridable).
    traefik_binary         TEXT NOT NULL DEFAULT '/usr/local/bin/traefik',
    config_dir             TEXT NOT NULL DEFAULT '/etc/hashi/traefik',
    api_port               INTEGER NOT NULL DEFAULT 8080 CHECK (api_port BETWEEN 1 AND 65535),
    -- What validation found before Hashi took over (binary, service, configs) as JSON; shown with the ownership step.
    discovery_json         TEXT NOT NULL DEFAULT '{}',
    ownership_confirmed_at INTEGER,
    installed_version      TEXT NOT NULL DEFAULT '',
    health                 TEXT NOT NULL DEFAULT 'unknown'
                           CHECK (health IN ('unknown', 'validating', 'healthy', 'degraded', 'failed')),
    last_validated_at      INTEGER,
    last_validation_error  TEXT NOT NULL DEFAULT '',
    created_at             INTEGER NOT NULL,
    updated_at             INTEGER NOT NULL,
    CHECK (auth = 'hashi_key' OR credential_secret_id IS NOT NULL)
) STRICT;

-- The hash of every file Hashi last wrote to a host (04 §6 step 5). Apply compares with the host's actual hash, not
-- only with this row; a path without a row was never written by Hashi.
CREATE TABLE traefik_files (
    connection_id     TEXT NOT NULL REFERENCES traefik_connections (id) ON DELETE CASCADE,
    path              TEXT NOT NULL,
    last_applied_hash TEXT NOT NULL,
    applied_at        INTEGER NOT NULL,
    PRIMARY KEY (connection_id, path)
) STRICT;

-- A Hashi-issued certificate (03 §5.3). The certificate chain is public; the private key is a vault secret.
-- names_json lists every SAN (root first); a certificate without cert_pem has not been issued yet.
CREATE TABLE certificates (
    id                  TEXT PRIMARY KEY,
    name                TEXT NOT NULL UNIQUE,
    names_json          TEXT NOT NULL,
    key_secret_id       TEXT REFERENCES secret_records (id),
    cert_pem            TEXT NOT NULL DEFAULT '',
    issuer              TEXT NOT NULL DEFAULT '',
    not_before          INTEGER,
    not_after           INTEGER,
    -- sha256 of the leaf certificate (DER), hex; Verify compares it with what each edge serves.
    fingerprint         TEXT NOT NULL DEFAULT '',
    last_renewal_at     INTEGER,
    last_renewal_result TEXT NOT NULL DEFAULT '' CHECK (last_renewal_result IN ('', 'issued', 'renewed', 'failed')),
    last_renewal_error  TEXT NOT NULL DEFAULT '',
    created_at          INTEGER NOT NULL,
    updated_at          INTEGER NOT NULL,
    CHECK (cert_pem = '' OR key_secret_id IS NOT NULL)
) STRICT;

-- Which certificate version each Traefik host has and serves (Traefik → Certificates tab, per-host state).
CREATE TABLE certificate_distributions (
    certificate_id   TEXT NOT NULL REFERENCES certificates (id) ON DELETE CASCADE,
    connection_id    TEXT NOT NULL REFERENCES traefik_connections (id) ON DELETE CASCADE,
    fingerprint      TEXT NOT NULL DEFAULT '',
    state            TEXT NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'applied', 'verified', 'failed')),
    last_verified_at INTEGER,
    error            TEXT NOT NULL DEFAULT '',
    updated_at       INTEGER NOT NULL,
    PRIMARY KEY (certificate_id, connection_id)
) STRICT;

-- sync_plans.subsystem gains traefik (install, files and certificate distribution per host) and certificates
-- (issue/renew). SQLite cannot change a CHECK in place: the three sync tables are rebuilt. The children are copied
-- into new tables first and the old ones dropped children-first, so no ON DELETE CASCADE fires on the way.
CREATE TABLE sync_plans_v5 (
    id          TEXT PRIMARY KEY,
    subsystem   TEXT NOT NULL CHECK (subsystem IN ('dns-records', 'dns-record-deletes', 'dns-prune', 'dns-probe',
                                                   'traefik', 'certificates')),
    target      TEXT NOT NULL,
    title       TEXT NOT NULL,
    trigger     TEXT NOT NULL CHECK (trigger IN ('save', 'reconcile', 'manual', 'probe', 'prune')),
    risk        TEXT NOT NULL CHECK (risk IN ('low', 'high')),
    status      TEXT NOT NULL CHECK (status IN ('pending', 'approved', 'applying', 'applied', 'failed', 'discarded', 'superseded')),
    plan_hash   TEXT NOT NULL,
    created_by  TEXT NOT NULL,
    created_at  INTEGER NOT NULL,
    approved_by TEXT NOT NULL DEFAULT '',
    approved_at INTEGER,
    finished_at INTEGER,
    result      TEXT NOT NULL DEFAULT '',
    error       TEXT NOT NULL DEFAULT ''
) STRICT;
INSERT INTO sync_plans_v5 SELECT * FROM sync_plans;

CREATE TABLE sync_plan_items_v5 (
    plan_id     TEXT NOT NULL REFERENCES sync_plans_v5 (id) ON DELETE CASCADE,
    seq         INTEGER NOT NULL,
    action      TEXT NOT NULL CHECK (action IN ('create', 'update', 'delete', 'label', 'conflict')),
    object      TEXT NOT NULL,
    before_json TEXT NOT NULL DEFAULT '',
    after_json  TEXT NOT NULL DEFAULT '',
    risk        TEXT NOT NULL CHECK (risk IN ('low', 'high')),
    note        TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (plan_id, seq)
) STRICT;
INSERT INTO sync_plan_items_v5 SELECT * FROM sync_plan_items;

CREATE TABLE sync_runs_v5 (
    id          TEXT PRIMARY KEY,
    plan_id     TEXT NOT NULL REFERENCES sync_plans_v5 (id) ON DELETE CASCADE,
    started_at  INTEGER NOT NULL,
    finished_at INTEGER,
    outcome     TEXT NOT NULL DEFAULT 'running'
                CHECK (outcome IN ('running', 'applied', 'failed', 'replanned', 'no_changes')),
    summary     TEXT NOT NULL DEFAULT '',
    error       TEXT NOT NULL DEFAULT ''
) STRICT;
INSERT INTO sync_runs_v5 SELECT * FROM sync_runs;

DROP TABLE sync_plan_items;
DROP TABLE sync_runs;
DROP TABLE sync_plans;
-- Renaming the parent rewrites the children's REFERENCES to the new name.
ALTER TABLE sync_plans_v5 RENAME TO sync_plans;
ALTER TABLE sync_plan_items_v5 RENAME TO sync_plan_items;
ALTER TABLE sync_runs_v5 RENAME TO sync_runs;
CREATE UNIQUE INDEX sync_plans_open ON sync_plans (subsystem, target) WHERE status IN ('pending', 'approved', 'applying');
CREATE INDEX sync_plans_created ON sync_plans (created_at);
CREATE INDEX sync_runs_started ON sync_runs (started_at);

-- +goose Down
DELETE FROM sync_plans WHERE subsystem IN ('traefik', 'certificates');
CREATE TABLE sync_plans_v4 (
    id          TEXT PRIMARY KEY,
    subsystem   TEXT NOT NULL CHECK (subsystem IN ('dns-records', 'dns-record-deletes', 'dns-prune', 'dns-probe')),
    target      TEXT NOT NULL,
    title       TEXT NOT NULL,
    trigger     TEXT NOT NULL CHECK (trigger IN ('save', 'reconcile', 'manual', 'probe', 'prune')),
    risk        TEXT NOT NULL CHECK (risk IN ('low', 'high')),
    status      TEXT NOT NULL CHECK (status IN ('pending', 'approved', 'applying', 'applied', 'failed', 'discarded', 'superseded')),
    plan_hash   TEXT NOT NULL,
    created_by  TEXT NOT NULL,
    created_at  INTEGER NOT NULL,
    approved_by TEXT NOT NULL DEFAULT '',
    approved_at INTEGER,
    finished_at INTEGER,
    result      TEXT NOT NULL DEFAULT '',
    error       TEXT NOT NULL DEFAULT ''
) STRICT;
INSERT INTO sync_plans_v4 SELECT * FROM sync_plans;
CREATE TABLE sync_plan_items_v4 (
    plan_id     TEXT NOT NULL REFERENCES sync_plans_v4 (id) ON DELETE CASCADE,
    seq         INTEGER NOT NULL,
    action      TEXT NOT NULL CHECK (action IN ('create', 'update', 'delete', 'label', 'conflict')),
    object      TEXT NOT NULL,
    before_json TEXT NOT NULL DEFAULT '',
    after_json  TEXT NOT NULL DEFAULT '',
    risk        TEXT NOT NULL CHECK (risk IN ('low', 'high')),
    note        TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (plan_id, seq)
) STRICT;
INSERT INTO sync_plan_items_v4 SELECT * FROM sync_plan_items;
CREATE TABLE sync_runs_v4 (
    id          TEXT PRIMARY KEY,
    plan_id     TEXT NOT NULL REFERENCES sync_plans_v4 (id) ON DELETE CASCADE,
    started_at  INTEGER NOT NULL,
    finished_at INTEGER,
    outcome     TEXT NOT NULL DEFAULT 'running'
                CHECK (outcome IN ('running', 'applied', 'failed', 'replanned', 'no_changes')),
    summary     TEXT NOT NULL DEFAULT '',
    error       TEXT NOT NULL DEFAULT ''
) STRICT;
INSERT INTO sync_runs_v4 SELECT * FROM sync_runs;
DROP TABLE sync_plan_items;
DROP TABLE sync_runs;
DROP TABLE sync_plans;
ALTER TABLE sync_plans_v4 RENAME TO sync_plans;
ALTER TABLE sync_plan_items_v4 RENAME TO sync_plan_items;
ALTER TABLE sync_runs_v4 RENAME TO sync_runs;
CREATE UNIQUE INDEX sync_plans_open ON sync_plans (subsystem, target) WHERE status IN ('pending', 'approved', 'applying');
CREATE INDEX sync_plans_created ON sync_plans (created_at);
CREATE INDEX sync_runs_started ON sync_runs (started_at);

DROP TABLE certificate_distributions;
DROP TABLE certificates;
DROP TABLE traefik_files;
DROP TABLE traefik_connections;
DROP TABLE acme_accounts;
