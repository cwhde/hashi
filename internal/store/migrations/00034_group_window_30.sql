-- +goose Up
-- 0.13 (13.5 gate, 2026-11-22): a real outage spread three services' alerts over 32 s, 21 s between the last two, so a
-- 15 s window only held them together by luck of the delivery job's timing. Checks run every 60 s, so alerts from one
-- outage arrive tens of seconds apart; 30 s keeps them in one message. The route is one row: only the old default moves.
UPDATE notification_route SET group_seconds = 30 WHERE id = 1 AND group_seconds = 15;

-- +goose Down
UPDATE notification_route SET group_seconds = 15 WHERE id = 1 AND group_seconds = 30;
