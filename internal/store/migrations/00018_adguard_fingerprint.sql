-- +goose Up
-- A connection may name the exact certificate it expects (03 §8): an instance with a self-signed certificate can then prove
-- who it is, so its password may follow a Pulse agent's address (08 §5).
ALTER TABLE adguard_connections ADD COLUMN tls_fingerprint TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE adguard_connections DROP COLUMN tls_fingerprint;
