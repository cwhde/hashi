-- +goose Up
-- 0.13.4: the Security dashboard's recent events read the whole table without an index on the time alone (bug 99).
CREATE INDEX security_events_occurred ON security_events (occurred_at);

-- +goose Down
DROP INDEX security_events_occurred;
