-- +goose Up
-- One name can mix protocols: gRPC needs h2c next to plain HTTP on the same host (04 §1.1). Empty uses the service's kind.
ALTER TABLE service_routes ADD COLUMN kind TEXT NOT NULL DEFAULT '' CHECK (kind IN ('', 'http', 'https', 'h2c'));

-- +goose Down
ALTER TABLE service_routes DROP COLUMN kind;
