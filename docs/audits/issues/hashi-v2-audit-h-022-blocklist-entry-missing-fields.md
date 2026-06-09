# H-022: BlocklistEntryEntity Missing first_seen_at_utc

**Priority:** Low
**Conflict Type:** missing_implementation
**Spec Reference:** Addendum §8.6

## Description

The Addendum specifies `blocklist_entries` with `first_seen_at_utc` and `last_seen_at_utc` fields. The `BlocklistEntryEntity` in `ExtendedPlatformEntities.cs` includes both:

```csharp
public DateTimeOffset FirstSeenAtUtc { get; set; } = DateTimeOffset.UtcNow;
public DateTimeOffset LastSeenAtUtc { get; set; } = DateTimeOffset.UtcNow;
```

This finding is a false positive - both fields are present.

## Expected Outcome

- Blocklist entries track first and last seen timestamps

## Fix Guidance

No changes needed - fields are present.

## Acceptance Criteria

- [x] first_seen_at_utc field exists (implemented)
- [x] last_seen_at_utc field exists (implemented)
