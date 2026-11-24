-- +goose Up
-- 1.2: one set of check settings per endpoint, the same for its internal and public check (07 section 2). Empty means
-- the default: GET, the target's own path, the Monitoring setting's codes, no text, every address family.
ALTER TABLE monitor_endpoints ADD COLUMN check_method TEXT NOT NULL DEFAULT '' CHECK (check_method IN ('', 'GET', 'HEAD', 'POST'));
ALTER TABLE monitor_endpoints ADD COLUMN check_path TEXT NOT NULL DEFAULT '';
ALTER TABLE monitor_endpoints ADD COLUMN check_codes TEXT NOT NULL DEFAULT '';
ALTER TABLE monitor_endpoints ADD COLUMN check_contains TEXT NOT NULL DEFAULT '';
ALTER TABLE monitor_endpoints ADD COLUMN check_family TEXT NOT NULL DEFAULT '' CHECK (check_family IN ('', '4', '6'));
ALTER TABLE monitor_endpoints ADD COLUMN check_skip_verify INTEGER NOT NULL DEFAULT 0 CHECK (check_skip_verify IN (0, 1));

-- +goose Down
ALTER TABLE monitor_endpoints DROP COLUMN check_skip_verify;
ALTER TABLE monitor_endpoints DROP COLUMN check_family;
ALTER TABLE monitor_endpoints DROP COLUMN check_contains;
ALTER TABLE monitor_endpoints DROP COLUMN check_codes;
ALTER TABLE monitor_endpoints DROP COLUMN check_path;
ALTER TABLE monitor_endpoints DROP COLUMN check_method;
