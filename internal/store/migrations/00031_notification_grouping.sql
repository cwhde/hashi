-- +goose Up
-- 0.13.4: alerts arriving close together go as one message (07 section 6, bug 144). The window restarts with each
-- further alert; 0 sends each at once.
ALTER TABLE notification_route ADD COLUMN group_seconds INTEGER NOT NULL DEFAULT 15 CHECK (group_seconds BETWEEN 0 AND 120);

-- +goose Down
ALTER TABLE notification_route DROP COLUMN group_seconds;
