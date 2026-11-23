-- +goose Up
-- 0.13.17: a flapping check sends one "keeps going down and up" message and one when it has settled (07 section 6,
-- bug 187). SQLite cannot change a CHECK in place, so the deliveries table is rebuilt; nothing points at it.
CREATE TABLE notification_deliveries_v36 (
    id              TEXT PRIMARY KEY,
    provider_id     TEXT NOT NULL REFERENCES notification_providers (id) ON DELETE CASCADE,
    endpoint_id     TEXT REFERENCES monitor_endpoints (id) ON DELETE SET NULL,
    kind            TEXT NOT NULL CHECK (kind IN ('alert', 'recovery', 'test', 'flapping', 'settled')),
    subject         TEXT NOT NULL,
    body            TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'delivered', 'failed')),
    attempts        INTEGER NOT NULL DEFAULT 0,
    next_attempt_at INTEGER NOT NULL,
    last_error      TEXT NOT NULL DEFAULT '',
    created_at      INTEGER NOT NULL,
    delivered_at    INTEGER
) STRICT;
INSERT INTO notification_deliveries_v36 SELECT * FROM notification_deliveries;
DROP TABLE notification_deliveries;
ALTER TABLE notification_deliveries_v36 RENAME TO notification_deliveries;
CREATE INDEX notification_deliveries_due ON notification_deliveries (status, next_attempt_at);
CREATE INDEX notification_deliveries_endpoint ON notification_deliveries (endpoint_id, created_at);

-- +goose Down
CREATE TABLE notification_deliveries_v35 (
    id              TEXT PRIMARY KEY,
    provider_id     TEXT NOT NULL REFERENCES notification_providers (id) ON DELETE CASCADE,
    endpoint_id     TEXT REFERENCES monitor_endpoints (id) ON DELETE SET NULL,
    kind            TEXT NOT NULL CHECK (kind IN ('alert', 'recovery', 'test')),
    subject         TEXT NOT NULL,
    body            TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'delivered', 'failed')),
    attempts        INTEGER NOT NULL DEFAULT 0,
    next_attempt_at INTEGER NOT NULL,
    last_error      TEXT NOT NULL DEFAULT '',
    created_at      INTEGER NOT NULL,
    delivered_at    INTEGER
) STRICT;
INSERT INTO notification_deliveries_v35 SELECT * FROM notification_deliveries WHERE kind IN ('alert', 'recovery', 'test');
DROP TABLE notification_deliveries;
ALTER TABLE notification_deliveries_v35 RENAME TO notification_deliveries;
CREATE INDEX notification_deliveries_due ON notification_deliveries (status, next_attempt_at);
CREATE INDEX notification_deliveries_endpoint ON notification_deliveries (endpoint_id, created_at);
