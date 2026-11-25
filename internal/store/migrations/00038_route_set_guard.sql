-- +goose Up
-- Bug 198: route targets live in service_routes.targets_json, which no foreign key can see. A set a route still uses
-- (an IP set, or a Pulse agent's hidden set) cannot be deleted, as if that JSON were an ON DELETE RESTRICT column.
-- +goose StatementBegin
CREATE TRIGGER ip_sets_used_by_route BEFORE DELETE ON ip_sets
WHEN EXISTS (SELECT 1 FROM service_routes r, json_each(r.targets_json) t WHERE json_extract(t.value, '$.set_id') = OLD.id)
BEGIN
    SELECT RAISE(ABORT, 'FOREIGN KEY constraint failed: a service route uses this set');
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER ip_sets_used_by_route;
