-- +goose Up
-- WAF mode, uploads and rule exclusions of a service and its paths (06 §9), edited only through PUT /api/services/{id}/waf.
ALTER TABLE services ADD COLUMN waf_json TEXT NOT NULL DEFAULT '{}';

-- The tail follows two files per Traefik host: the access log and the general log, where the WAF's matches are (06 §6.2).
CREATE TABLE access_log_cursors_new (
    traefik_connection_id TEXT NOT NULL REFERENCES traefik_connections (id) ON DELETE CASCADE,
    file                  TEXT NOT NULL CHECK (file IN ('access.log', 'traefik.log')),
    inode                 INTEGER NOT NULL,
    offset                INTEGER NOT NULL,
    lag_ms                INTEGER NOT NULL DEFAULT 0,
    last_read_at          INTEGER,
    error                 TEXT NOT NULL DEFAULT '',
    updated_at            INTEGER NOT NULL,
    PRIMARY KEY (traefik_connection_id, file)
) STRICT;
INSERT INTO access_log_cursors_new (traefik_connection_id, file, inode, offset, lag_ms, last_read_at, error, updated_at)
SELECT traefik_connection_id, 'access.log', inode, offset, lag_ms, last_read_at, error, updated_at FROM access_log_cursors;
DROP TABLE access_log_cursors;
ALTER TABLE access_log_cursors_new RENAME TO access_log_cursors;

-- +goose Down
CREATE TABLE access_log_cursors_old (
    traefik_connection_id TEXT PRIMARY KEY REFERENCES traefik_connections (id) ON DELETE CASCADE,
    inode                 INTEGER NOT NULL,
    offset                INTEGER NOT NULL,
    lag_ms                INTEGER NOT NULL DEFAULT 0,
    last_read_at          INTEGER,
    error                 TEXT NOT NULL DEFAULT '',
    updated_at            INTEGER NOT NULL
) STRICT;
INSERT INTO access_log_cursors_old (traefik_connection_id, inode, offset, lag_ms, last_read_at, error, updated_at)
SELECT traefik_connection_id, inode, offset, lag_ms, last_read_at, error, updated_at FROM access_log_cursors
WHERE file = 'access.log';
DROP TABLE access_log_cursors;
ALTER TABLE access_log_cursors_old RENAME TO access_log_cursors;
ALTER TABLE services DROP COLUMN waf_json;
