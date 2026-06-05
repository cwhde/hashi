# G02 - Addendum background jobs are registered but not run

Priority: High

Spec conflicts: addendum section 13

## Problem

The addendum requires five operational background jobs for blocklist fetching, security bucket aggregation, block expiry, internal agent DNS sync, and challenge cleanup. The implementation creates `background_jobs` rows for those keys, but it does not register hosted workers or equivalent schedulers that execute those responsibilities.

As a result, enabled blocklist sources are not refreshed automatically, expired soft/firewall blocks and abandoned challenges are not cleaned up by a job, and internal agent DNS does not have the required periodic sync job. The jobs can appear present in the database while their required behavior never happens.

## Evidence

- `docs/implementation-spec/hashi-v2-implementation-spec-addendum.md:1103` says to add background jobs.
- `docs/implementation-spec/hashi-v2-implementation-spec-addendum.md:1107` through `docs/implementation-spec/hashi-v2-implementation-spec-addendum.md:1155` define the required responsibilities for blocklist fetch, security bucket aggregation, block expiry, internal agent DNS sync, and challenge cleanup.
- `src/Hashi.Infrastructure/Platform/BackgroundJobService.cs:15` through `src/Hashi.Infrastructure/Platform/BackgroundJobService.cs:19` define keys for these addendum jobs.
- `src/Hashi.Infrastructure/Platform/BackgroundJobService.cs:32` through `src/Hashi.Infrastructure/Platform/BackgroundJobService.cs:36` only ensure database job rows for those keys.
- `src/Hashi.Infrastructure/DependencyInjection.cs:118` through `src/Hashi.Infrastructure/DependencyInjection.cs:123` register hosted workers for monitor checks, rollups, passive sync, scripts, access-log ingest, and GeoIP updates, but not for the five addendum jobs.
- `tests/Hashi.UnitTests/BackgroundJobServiceTests.cs:21` through `tests/Hashi.UnitTests/BackgroundJobServiceTests.cs:25` assert that the keys are registered, not that any worker executes the responsibilities.

## Expected outcome

Each addendum job should have an executable background worker or scheduler path that updates its `background_jobs` row, performs the required work, records success/failure, and is covered by tests.

## Fix guidance

Implement hosted services or fold these jobs into an existing scheduler with explicit job handlers. The handlers should:

- Fetch due enabled blocklist sources and queue firewall sync when needed.
- Perform any needed security bucket/dashboard aggregation.
- Expire soft and firewall blocks and record expiry events.
- Periodically sync internal agent DNS when enabled.
- Clean up stale CAPTCHA challenge states and abandoned attempts.

## Acceptance criteria

- The five addendum jobs are executed automatically according to their configured cadence.
- Job status, last run timestamps, and errors are persisted.
- Blocklist refresh, block expiry, internal DNS sync, and challenge cleanup have unit or integration coverage.
- The app no longer only creates idle metadata rows for these jobs.
