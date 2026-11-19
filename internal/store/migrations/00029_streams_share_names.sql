-- +goose Up
-- 0.13.4: a TCP/UDP stream may share its name with an HTTP service (a game server and its web map, bug 132). Only HTTP
-- services are routed by name, so only they must be unique per name.
DROP INDEX services_host_name;
CREATE UNIQUE INDEX services_host_name ON services (host_name) WHERE host_name != '' AND kind NOT IN ('tcp', 'udp');

-- +goose Down
DROP INDEX services_host_name;
CREATE UNIQUE INDEX services_host_name ON services (host_name) WHERE host_name != '';
