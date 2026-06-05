# G06 - Blocklist refresh loses entry lifecycle and fetch metadata

Priority: Medium

Spec conflicts: addendum sections 8.5, 8.6, and 8.8

## Problem

The addendum requires blocklist sources, entries, and fetch runs to track detailed lifecycle metadata, including source success/error/http counters, entry first/last-seen timestamps, and rejected entry counts. The implemented model exposes a reduced set of fields, and successful refresh deletes all old entries for a source before inserting the newly parsed entries.

That delete/reinsert approach means unchanged entries do not keep a stable row identity, cannot preserve first-seen/last-seen metadata, and can drop per-entry applied-host state through cascade behavior. Parse rejections are only stored inside run metadata JSON, not in the required source/run fields.

## Evidence

- `docs/implementation-spec/hashi-v2-implementation-spec-addendum.md:691` through `docs/implementation-spec/hashi-v2-implementation-spec-addendum.md:694` requires parse errors, ignored entries, preview-before-enable, and status recording.
- `docs/implementation-spec/hashi-v2-implementation-spec-addendum.md:698` through `docs/implementation-spec/hashi-v2-implementation-spec-addendum.md:720` requires blocklist source fields including last success, last HTTP status, entry count, and rejected count.
- `docs/implementation-spec/hashi-v2-implementation-spec-addendum.md:724` through `docs/implementation-spec/hashi-v2-implementation-spec-addendum.md:737` requires blocklist entries to include `first_seen_at_utc` and `last_seen_at_utc`.
- `docs/implementation-spec/hashi-v2-implementation-spec-addendum.md:745` through `docs/implementation-spec/hashi-v2-implementation-spec-addendum.md:762` requires fetch runs to include parsed/added/removed/unchanged/rejected counts.
- `src/Hashi.Infrastructure/Persistence/Entities/ExtendedPlatformEntities.cs:623` through `src/Hashi.Infrastructure/Persistence/Entities/ExtendedPlatformEntities.cs:662` defines `BlocklistEntryEntity` without first-seen or last-seen fields.
- `src/Hashi.Infrastructure/Persistence/Entities/ExtendedPlatformEntities.cs:664` through `src/Hashi.Infrastructure/Persistence/Entities/ExtendedPlatformEntities.cs:709` defines `BlocklistSourceEntity` with `LastFetchedAtUtc` and `LastFetchStatus`, but without the required last-success, last HTTP status, entry count, or rejected count fields.
- `src/Hashi.Infrastructure/Persistence/Entities/ExtendedPlatformEntities.cs:711` through `src/Hashi.Infrastructure/Persistence/Entities/ExtendedPlatformEntities.cs:743` defines `BlocklistFetchRunEntity` without an explicit rejected-entry count.
- `src/Hashi.Infrastructure/Platform/BlocklistSourceManagementService.cs:247` through `src/Hashi.Infrastructure/Platform/BlocklistSourceManagementService.cs:251` removes all existing entries for the source and inserts new entities on each successful refresh.
- `src/Hashi.Infrastructure/Platform/BlocklistSourceManagementService.cs:253` through `src/Hashi.Infrastructure/Platform/BlocklistSourceManagementService.cs:259` computes unchanged counts, but those unchanged entries have already been scheduled for replacement.
- `src/Hashi.Infrastructure/Persistence/HashiDbContext.cs:855` configures `BlocklistAppliedHostEntity` to cascade when a `BlocklistEntryEntity` is deleted, so refresh can remove applied-host state for unchanged entries.

## Expected outcome

Blocklist refresh should preserve stable entry rows for unchanged subjects, maintain first-seen/last-seen metadata, expose source and run counters required by the addendum, and retain applied-host state for unchanged entries.

## Fix guidance

Add the missing source, entry, and fetch-run fields. Change refresh to perform a merge/upsert:

- Insert new entries with `first_seen_at_utc` and `last_seen_at_utc`.
- Update existing entries' `last_seen_at_utc`, metadata, enabled state, and enforcement mode without changing their IDs.
- Disable or remove entries that disappeared while preserving enough history if needed.
- Store rejected counts explicitly on fetch runs and source summaries.
- Avoid deleting unchanged entries and their applied-host records.

## Acceptance criteria

- Blocklist entries persist `first_seen_at_utc` and `last_seen_at_utc`.
- Successful refresh preserves row IDs for unchanged entries.
- Applied-host status for unchanged firewall-enforced entries survives refresh.
- Source responses expose required success/http/count metadata.
- Fetch run responses expose rejected entry counts.
- Tests cover a refresh where one entry is added, one removed, and one unchanged.
