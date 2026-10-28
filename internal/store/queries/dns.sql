-- name: InsertDNSProvider :exec
INSERT INTO dns_providers (id, name, type, default_ttl, token_secret_id, health, last_validated_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateDNSProvider :exec
UPDATE dns_providers
SET default_ttl = ?, token_secret_id = ?, health = ?, last_validated_at = ?, last_validation_error = ?, updated_at = ?
WHERE id = ?;

-- name: SetDNSProviderHealth :exec
UPDATE dns_providers SET health = ?, last_validated_at = ?, last_validation_error = ?, updated_at = ? WHERE id = ?;

-- name: GetDNSProvider :one
SELECT * FROM dns_providers WHERE id = ?;

-- name: ListDNSProviders :many
SELECT * FROM dns_providers ORDER BY created_at, id;

-- name: InsertDNSZone :exec
INSERT INTO dns_zones (id, provider_id, provider_zone_id, name, zone_ttl, created_at) VALUES (?, ?, ?, ?, ?, ?);

-- name: DeleteDNSZonesExcept :execrows
-- Hashi manages one zone (setup step 3): choosing another one drops the old zone with its snapshot, ownership and decisions.
DELETE FROM dns_zones WHERE provider_id = ? AND id != ?;

-- name: GetDNSZone :one
SELECT * FROM dns_zones WHERE id = ?;

-- name: GetDNSZoneByProviderZone :one
SELECT * FROM dns_zones WHERE provider_id = ? AND provider_zone_id = ?;

-- name: ListDNSZones :many
SELECT * FROM dns_zones ORDER BY name;

-- name: SetDNSZoneRefreshed :exec
UPDATE dns_zones SET zone_ttl = ?, last_refreshed_at = ?, last_refresh_error = '', rrset_count = ? WHERE id = ?;

-- name: SetDNSZoneRefreshError :exec
UPDATE dns_zones SET last_refresh_error = ? WHERE id = ?;

-- name: ListObservedRRSets :many
SELECT * FROM dns_observed_rrsets WHERE zone_id = ? ORDER BY name, type;

-- name: DeleteObservedRRSets :exec
DELETE FROM dns_observed_rrsets WHERE zone_id = ?;

-- name: InsertObservedRRSet :exec
INSERT INTO dns_observed_rrsets (zone_id, name, type, ttl, values_json, labels_json, protected, hash, observed_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListDNSOwnership :many
SELECT * FROM dns_ownership WHERE zone_id = ? ORDER BY name, type;

-- name: UpsertDNSOwnership :exec
-- Never downgrades: an RRSet already owned with another origin keeps it.
INSERT INTO dns_ownership (zone_id, name, type, origin, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (zone_id, name, type) DO NOTHING;

-- name: DeleteImportedOwnership :execrows
-- Only an import decision taken back removes ownership, and only ownership that import created.
DELETE FROM dns_ownership WHERE zone_id = ? AND name = ? AND type = ? AND origin = 'imported';

-- name: ListImportDecisions :many
SELECT * FROM dns_import_decisions WHERE zone_id = ? ORDER BY name, type;

-- name: UpsertImportDecision :exec
INSERT INTO dns_import_decisions (zone_id, name, type, decision, decided_at) VALUES (?, ?, ?, ?, ?)
ON CONFLICT (zone_id, name, type) DO UPDATE SET decision = excluded.decision, decided_at = excluded.decided_at;

-- name: ListDNSRecords :many
SELECT * FROM dns_records WHERE zone_id = ? ORDER BY name, type;

-- name: GetDNSRecord :one
SELECT * FROM dns_records WHERE id = ?;

-- name: GetDNSRecordByKey :one
SELECT * FROM dns_records WHERE zone_id = ? AND name = ? AND type = ?;

-- name: InsertDNSRecord :exec
INSERT INTO dns_records (id, zone_id, name, type, ttl, values_json, origin, display_name, dashboard, monitoring, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateDNSRecord :exec
UPDATE dns_records SET ttl = ?, values_json = ?, display_name = ?, dashboard = ?, monitoring = ?, updated_at = ? WHERE id = ?;

-- name: DeleteDNSRecord :exec
DELETE FROM dns_records WHERE id = ?;

-- name: DeleteImportedDNSRecord :exec
DELETE FROM dns_records WHERE zone_id = ? AND name = ? AND type = ? AND origin = 'imported';

-- name: DeleteDNSOwnership :exec
DELETE FROM dns_ownership WHERE zone_id = ? AND name = ? AND type = ?;

-- name: SetDNSOwnershipApplied :exec
UPDATE dns_ownership SET last_applied_hash = ?, updated_at = ? WHERE zone_id = ? AND name = ? AND type = ?;

-- name: DeleteImportDecision :exec
DELETE FROM dns_import_decisions WHERE zone_id = ? AND name = ? AND type = ?;
