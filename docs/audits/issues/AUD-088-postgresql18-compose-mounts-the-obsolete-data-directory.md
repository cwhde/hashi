# AUD-088: PostgreSQL18 Compose mounts the obsolete data directory

Priority: P1

Category: Deployment / persistence

Status: open; confirmed by manual source tracing.

## Finding

Both production and development Compose select postgres:18 but mount their named volume at /var/lib/postgresql/data. PostgreSQL18 defaults PGDATA to /var/lib/postgresql/18/docker and declares the volume at /var/lib/postgresql. The configured named volume therefore does not contain the actual database; current official image entrypoint also detects the obsolete mount and can refuse startup. This prevents reliable first installation and persistence across container recreation.

## Evidence

deploy/compose/docker-compose.yml:3,8-9; deploy/compose/docker-compose.dev.yml:3,8-9

[Official PostgreSQL image documentation](https://github.com/docker-library/docs/blob/master/postgres/content.md).

## Expected outcome

Mount the PostgreSQL18 parent path and provide an explicit migration/backup procedure for any existing named volume; never silently initialize over existing data.

## Acceptance criteria

A fresh Compose install starts PostgreSQL18, actual PGDATA is inside the intended named volume, and database contents survive container replacement. An existing17/18 data directory has an explicit supported recovery path.
