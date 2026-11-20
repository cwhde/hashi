-- +goose Up
-- 0.13.8: each dashboard app's icon (07 section 9.1, bug 149). One row per app with a chosen icon or a fetch result; an
-- app without a row is automatic and not fetched yet. The files live in <data>/assets/apps/, named by app.
CREATE TABLE app_icons (
    app           TEXT PRIMARY KEY,
    service_id    TEXT UNIQUE REFERENCES services (id) ON DELETE CASCADE,
    dns_record_id TEXT UNIQUE REFERENCES dns_records (id) ON DELETE CASCADE,
    mode          TEXT NOT NULL CHECK (mode IN ('auto', 'upload', 'library')),
    library_ref   TEXT NOT NULL DEFAULT '',
    content_type  TEXT NOT NULL DEFAULT '',
    etag          TEXT NOT NULL DEFAULT '',
    light_type    TEXT NOT NULL DEFAULT '',
    light_etag    TEXT NOT NULL DEFAULT '',
    found         TEXT NOT NULL DEFAULT '',
    checked_at    INTEGER NOT NULL DEFAULT 0,
    CHECK ((service_id IS NULL) <> (dns_record_id IS NULL))
) STRICT;

-- +goose Down
DROP TABLE app_icons;
