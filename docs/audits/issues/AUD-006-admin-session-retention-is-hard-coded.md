# AUD-006 - Revoked-session retention is hard-coded

Priority: Low (P3)

Category: Configuration gap / session audit retention

Specification reference: `docs/implementation-spec/hashi-v2-admin-session-token-management-addendum.md`, section 15 (cleanup and retention).

## Problem

The session cleanup job deletes revoked session records after a fixed seven days. The addendum specifies retention according to the configured audit-retention interval. Operators cannot align session-record cleanup with their audit retention policy.

## Evidence

- `AdminSessionService.CleanupAsync` sets `retentionCutoff = now.AddDays(-7)` directly.
- The admin session settings and cleanup worker do not read an audit-retention value.

## Expected outcome

Revoked session records are retained according to the configured audit retention policy, with a documented default and safe minimum/maximum values.

## Acceptance criteria

- Use the existing audit-retention setting if present, or add one shared retention setting rather than a session-only duplicate.
- Validate allowed retention values and cover cleanup boundaries with tests.
- Preserve prompt removal of expired/revoked sessions from authentication and vault state regardless of database-history retention.
