-- +goose Up
-- The user middleware file 30-user-middlewares.yml (04 §4): one row, the text as the user typed it (comments kept).
CREATE TABLE traefik_user_middlewares (
    id         INTEGER PRIMARY KEY CHECK (id = 1),
    yaml       TEXT NOT NULL,
    updated_at INTEGER NOT NULL
) STRICT;

-- Traffic options (04 §1.1a) and extra middlewares of a service and of a route, edited with the service.
ALTER TABLE services ADD COLUMN traffic_json TEXT NOT NULL DEFAULT '{}';
ALTER TABLE services ADD COLUMN middlewares_json TEXT NOT NULL DEFAULT '[]';
ALTER TABLE service_routes ADD COLUMN middlewares_json TEXT NOT NULL DEFAULT '[]';

-- +goose Down
ALTER TABLE service_routes DROP COLUMN middlewares_json;
ALTER TABLE services DROP COLUMN middlewares_json;
ALTER TABLE services DROP COLUMN traffic_json;
DROP TABLE traefik_user_middlewares;
