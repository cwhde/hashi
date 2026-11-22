-- +goose Up
-- 0.13.12: one layout for both public pages (07 section 7): named groups in order and every entry's place. The Apps
-- and Status switches stay on the service, the DNS entry and the endpoint. Starts empty: the monitor's first source
-- sync fills it from the endpoints' groups and the old category order (internal/monitor/layout.go) and sets seeded.
CREATE TABLE public_layout (
    id      INTEGER PRIMARY KEY CHECK (id = 1),
    version INTEGER NOT NULL DEFAULT 0,
    seeded  INTEGER NOT NULL DEFAULT 0 CHECK (seeded IN (0, 1))
) STRICT;
INSERT INTO public_layout (id) VALUES (1);

CREATE TABLE public_groups (
    id       TEXT PRIMARY KEY,
    name     TEXT NOT NULL UNIQUE CHECK (name <> '' AND name <> 'Other'),
    position INTEGER NOT NULL
) STRICT;

-- subject_type: 'service' (services.id), 'dns' (dns_records.id) or 'endpoint' (monitor_endpoints.id of an endpoint
-- that belongs to neither). group_id NULL is "Other". position orders all entries; a group shows its own in that order.
CREATE TABLE public_entries (
    subject_type TEXT NOT NULL CHECK (subject_type IN ('service', 'dns', 'endpoint')),
    subject_id   TEXT NOT NULL,
    group_id     TEXT REFERENCES public_groups (id) ON DELETE SET NULL,
    position     INTEGER NOT NULL,
    PRIMARY KEY (subject_type, subject_id)
) STRICT;

-- +goose Down
DROP TABLE public_entries;
DROP TABLE public_groups;
DROP TABLE public_layout;
