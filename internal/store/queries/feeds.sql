-- name: InsertFeed :exec
INSERT INTO ip_set_feeds (ip_set_id, url, format, options_json, allow_http, catalog_key, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetFeed :one
SELECT * FROM ip_set_feeds WHERE ip_set_id = ?;

-- name: ListFeeds :many
SELECT * FROM ip_set_feeds ORDER BY ip_set_id;

-- name: SetFeedResult :exec
UPDATE ip_set_feeds SET etag = ?, last_modified = ?, content_hash = ?, entry_count = ?, rejected_count = ?,
    last_attempt_at = ?, last_success_at = ?, last_error = ?, updated_at = ? WHERE ip_set_id = ?;

-- name: SetFeedFailure :exec
UPDATE ip_set_feeds SET last_attempt_at = ?, last_error = ?, updated_at = ? WHERE ip_set_id = ?;

-- name: InsertFeedRun :exec
INSERT INTO ip_set_feed_runs (id, ip_set_id, started_at, finished_at, outcome, error, parsed, added, removed, unchanged,
    rejected, content_hash) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListFeedRuns :many
SELECT * FROM ip_set_feed_runs WHERE ip_set_id = ? ORDER BY started_at DESC, id DESC LIMIT 50;

-- name: PruneFeedRuns :exec
DELETE FROM ip_set_feed_runs WHERE ip_set_feed_runs.ip_set_id = sqlc.arg(set_id) AND ip_set_feed_runs.id NOT IN (
    SELECT r.id FROM ip_set_feed_runs AS r WHERE r.ip_set_id = sqlc.arg(set_id) ORDER BY r.started_at DESC, r.id DESC LIMIT 50);

-- name: DeleteFeedElements :exec
DELETE FROM ip_set_elements WHERE ip_set_id = ? AND origin = 'feed';
