-- +goose Up
-- S2: DNS provider connection, the managed zone, observed RRSets, ownership, import decisions (03-dns.md §1–4).
-- Timestamps are integer Unix milliseconds UTC (invariant 6), IDs are ULID text.

-- A DNS provider connection (01 §4.2 connection fields). The token lives only in the vault (secret_records).
CREATE TABLE dns_providers (
    id                   TEXT PRIMARY KEY,
    name                 TEXT NOT NULL,
    type                 TEXT NOT NULL CHECK (type IN ('hetzner')),
    enabled              INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    -- TTL Hashi uses for RRSets it creates (setup step 3).
    default_ttl          INTEGER NOT NULL CHECK (default_ttl BETWEEN 60 AND 86400),
    token_secret_id      TEXT NOT NULL REFERENCES secret_records (id),
    health               TEXT NOT NULL DEFAULT 'unknown'
                         CHECK (health IN ('unknown', 'validating', 'healthy', 'degraded', 'failed')),
    last_validated_at    INTEGER,
    last_validation_error TEXT NOT NULL DEFAULT '',
    deletion_policy      TEXT NOT NULL DEFAULT 'required'
                         CHECK (deletion_policy IN ('required', 'optional', 'system_linked', 'safe_to_remove')),
    created_at           INTEGER NOT NULL,
    updated_at           INTEGER NOT NULL
) STRICT;

-- The zone Hashi manages (one, 02 §1.3); the table stays zone-aware (03 §1).
CREATE TABLE dns_zones (
    id                 TEXT PRIMARY KEY,
    provider_id        TEXT NOT NULL REFERENCES dns_providers (id) ON DELETE CASCADE,
    provider_zone_id   TEXT NOT NULL,
    name               TEXT NOT NULL,
    -- The zone's own default TTL as the provider reports it (RRSets with ttl NULL inherit it).
    zone_ttl           INTEGER NOT NULL,
    last_refreshed_at  INTEGER,
    last_refresh_error TEXT NOT NULL DEFAULT '',
    rrset_count        INTEGER NOT NULL DEFAULT 0,
    created_at         INTEGER NOT NULL,
    UNIQUE (provider_id, provider_zone_id),
    UNIQUE (name)
) STRICT;

-- The last snapshot read from the provider. Replaced as a whole on every refresh.
-- name is relative to the zone ('@' = apex); ttl NULL = zone default; values sorted, TXT decoded (internal/dns).
CREATE TABLE dns_observed_rrsets (
    zone_id     TEXT NOT NULL REFERENCES dns_zones (id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    type        TEXT NOT NULL,
    ttl         INTEGER,
    values_json TEXT NOT NULL,
    labels_json TEXT NOT NULL DEFAULT '{}',
    -- The provider forbids changes (Hetzner protection.change): read-only to Hashi (03 §3).
    protected   INTEGER NOT NULL DEFAULT 0 CHECK (protected IN (0, 1)),
    -- sha256 of the normalized RRSet; detects changes between refreshes.
    hash        TEXT NOT NULL,
    observed_at INTEGER NOT NULL,
    PRIMARY KEY (zone_id, name, type)
) STRICT;

-- Ownership is authoritative here (invariant 1). An RRSet without a row is not Hashi's.
CREATE TABLE dns_ownership (
    zone_id           TEXT NOT NULL REFERENCES dns_zones (id) ON DELETE CASCADE,
    name              TEXT NOT NULL,
    type              TEXT NOT NULL CHECK (type NOT IN ('NS', 'SOA')), -- invariant 2
    origin            TEXT NOT NULL CHECK (origin IN ('system', 'managed', 'imported', 'user')),
    last_applied_hash TEXT NOT NULL DEFAULT '',
    created_at        INTEGER NOT NULL,
    updated_at        INTEGER NOT NULL,
    PRIMARY KEY (zone_id, name, type)
) STRICT;

-- Import decisions (03 §4). Undecided RRSets have no row and are left unmanaged.
CREATE TABLE dns_import_decisions (
    zone_id    TEXT NOT NULL REFERENCES dns_zones (id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    type       TEXT NOT NULL,
    decision   TEXT NOT NULL CHECK (decision IN ('import', 'leave', 'prune')),
    decided_at INTEGER NOT NULL,
    PRIMARY KEY (zone_id, name, type),
    -- NS/SOA can only be left alone (invariant 2).
    CHECK (type NOT IN ('NS', 'SOA') OR decision = 'leave')
) STRICT;

-- +goose Down
DROP TABLE dns_import_decisions;
DROP TABLE dns_ownership;
DROP TABLE dns_observed_rrsets;
DROP TABLE dns_zones;
DROP TABLE dns_providers;
