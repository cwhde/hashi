-- name: ListAppIcons :many
SELECT * FROM app_icons ORDER BY app;

-- name: GetAppIcon :one
SELECT * FROM app_icons WHERE app = ?;

-- name: UpsertAppIcon :exec
INSERT INTO app_icons (app, service_id, dns_record_id, mode, library_ref, content_type, etag, light_type, light_etag, found, checked_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (app) DO UPDATE SET mode = excluded.mode, library_ref = excluded.library_ref,
    content_type = excluded.content_type, etag = excluded.etag, light_type = excluded.light_type,
    light_etag = excluded.light_etag, found = excluded.found, checked_at = excluded.checked_at;
