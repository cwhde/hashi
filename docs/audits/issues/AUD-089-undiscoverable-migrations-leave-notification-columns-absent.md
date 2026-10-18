# AUD-089: Undiscoverable migrations leave notification columns absent and hide dormant view SQL

Priority: P1

Category: Persistence / migration discovery

Status: open; confirmed by manual source tracing.

## Finding

Four migration classes have neither MigrationAttribute nor DbContextAttribute in any partial file. EF migration discovery omits them. Two omitted classes duplicate changes made by later registered migrations, but AddNotificationRouteCooldownAndRecovery is the only actual migration adding CooldownMinutes and SendRecovery; later Designer models merely describe these fields. A database built by Migrate consequently lacks fields selected by the notification route entity and route operations fail. AddMonitorDataViews is also undiscoverable. These SQL views are not mandatory: the specification uses views for product time ranges, and current APIs query raw samples/rollups. Their absence is not a missing-feature finding. Its SQL additionally references unquoted mixed-case columns, which PostgreSQL folds to lowercase; simply adding attributes would fail. Adding attributes indiscriminately to the two duplicate migrations would create duplicate-column errors.

## Evidence

src/Hashi.Infrastructure/Persistence/Migrations/20260604090000_AddNotificationRouteCooldownAndRecovery.cs:5-25; 20260604143000_AddCaptchaExpectationFlags.cs; 20260610100000_AddMonitorEndpointGroupAndConfigFields.cs; 20260610100100_AddMonitorDataViews.cs:5-54; 20260605083015_AddAddendumModelSync.cs:13-25; 20260610065242_FixPendingModelChanges.cs:98-141

Discovery source inventory in work/migration_discovery_inspect.py supplements the manual review; no tests were used as proof.

## Expected outcome

Repair discovery/history with forward migrations compatible with existing databases, add missing notification columns; either remove the unused view migration or repair its SQL if explicitly retained, and avoid replaying duplicate additions.

## Acceptance criteria

Migrate an empty PostgreSQL database and a representative earlier version. Compare actual columns to runtime model; notification route reads/writes work. If the optional SQL views are retained, verify all three execute. Do not add SQL views solely to satisfy the spec.
