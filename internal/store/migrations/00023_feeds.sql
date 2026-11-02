-- +goose Up
-- Feeds (06 §8): an IP set of kind feed gets its entries from a downloaded blocklist. The set's elements are the last
-- good download; a failed refresh keeps them and records why.
CREATE TABLE ip_set_feeds (
    ip_set_id       TEXT PRIMARY KEY REFERENCES ip_sets (id) ON DELETE CASCADE,
    url             TEXT NOT NULL,
    format          TEXT NOT NULL CHECK (format IN ('lines', 'csv', 'json_array', 'json_object', 'json_lines')),
    options_json    TEXT NOT NULL DEFAULT '{}',
    allow_http      INTEGER NOT NULL DEFAULT 0 CHECK (allow_http IN (0, 1)),
    catalog_key     TEXT NOT NULL DEFAULT '',
    etag            TEXT NOT NULL DEFAULT '',
    last_modified   TEXT NOT NULL DEFAULT '',
    content_hash    TEXT NOT NULL DEFAULT '',
    entry_count     INTEGER NOT NULL DEFAULT 0,
    rejected_count  INTEGER NOT NULL DEFAULT 0,
    last_attempt_at INTEGER,
    last_success_at INTEGER,
    last_error      TEXT NOT NULL DEFAULT '',
    created_at      INTEGER NOT NULL,
    updated_at      INTEGER NOT NULL
) STRICT;

-- One refresh. The newest 50 per feed are kept.
CREATE TABLE ip_set_feed_runs (
    id           TEXT PRIMARY KEY,
    ip_set_id    TEXT NOT NULL REFERENCES ip_sets (id) ON DELETE CASCADE,
    started_at   INTEGER NOT NULL,
    finished_at  INTEGER NOT NULL,
    outcome      TEXT NOT NULL CHECK (outcome IN ('updated', 'unchanged', 'failed', 'stale')),
    error        TEXT NOT NULL DEFAULT '',
    parsed       INTEGER NOT NULL DEFAULT 0,
    added        INTEGER NOT NULL DEFAULT 0,
    removed      INTEGER NOT NULL DEFAULT 0,
    unchanged    INTEGER NOT NULL DEFAULT 0,
    rejected     INTEGER NOT NULL DEFAULT 0,
    content_hash TEXT NOT NULL DEFAULT ''
) STRICT;
CREATE INDEX ip_set_feed_runs_set ON ip_set_feed_runs (ip_set_id, started_at DESC);

-- +goose Down
DROP TABLE ip_set_feed_runs;
DROP TABLE ip_set_feeds;
