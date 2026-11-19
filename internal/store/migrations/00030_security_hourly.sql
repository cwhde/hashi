-- +goose Up
-- 0.13.4: hourly rollups of the security minutes (01 section 4.3). The 7 and 30 day dashboards read these instead of
-- millions of minute rows (bug 99); the minutes go after 30 days, the hours after a year. No path prefix: nothing
-- reads it past the minute.
CREATE TABLE security_buckets_1h (
    bucket_start           INTEGER NOT NULL,
    service_id             TEXT NOT NULL DEFAULT '',
    root_domain            TEXT NOT NULL DEFAULT '',
    traefik_connection_id  TEXT NOT NULL DEFAULT '',
    country                TEXT NOT NULL DEFAULT '',
    asn                    INTEGER NOT NULL DEFAULT 0,
    decision               TEXT NOT NULL,
    status_class           TEXT NOT NULL DEFAULT '',
    request_count          INTEGER NOT NULL DEFAULT 0,
    denied_count           INTEGER NOT NULL DEFAULT 0,
    challenged_count       INTEGER NOT NULL DEFAULT 0,
    challenge_failed_count INTEGER NOT NULL DEFAULT 0,
    banned_count           INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (bucket_start, service_id, root_domain, traefik_connection_id, country, asn, decision, status_class)
) STRICT, WITHOUT ROWID;

-- The most active addresses of each hour (at most 500, like each minute).
CREATE TABLE security_ip_top_1h (
    bucket_start     INTEGER NOT NULL,
    address          TEXT NOT NULL,
    service_id       TEXT NOT NULL DEFAULT '',
    request_count    INTEGER NOT NULL DEFAULT 0,
    denied_count     INTEGER NOT NULL DEFAULT 0,
    challenged_count INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (bucket_start, address, service_id)
) STRICT, WITHOUT ROWID;

-- +goose Down
DROP TABLE security_ip_top_1h;
DROP TABLE security_buckets_1h;
