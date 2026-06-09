# H-049: SecurityAddendumJobWorker ExpireBlocksAsync N+1 Query Pattern

**Priority:** Medium
**Conflict Type:** bad_implementation
**Spec Reference:** Main Spec §4, Addendum §13.3

## Description

`SecurityAddendumJobWorker.ExpireBlocksAsync` in `src/Hashi.Infrastructure/Platform/SecurityAddendumJobWorker.cs` performs N+1 database queries when expiring blocks. For each expired subject, it:

1. Fetches the individual subject (1 query)
2. Checks manual entries per subject state (N queries for N subjects)
3. Updates each subject individually

```csharp
// SecurityAddendumJobWorker.cs — ExpireBlocksAsync (line ~194)
foreach (var entry in expiredEntries)
{
    var subject = await db.SecuritySubjects.FindAsync(entry.SecuritySubjectId); // N queries
    // ... check manual entries (more per-subject queries)
}
```

With hundreds or thousands of expired blocks, this creates significant database load. The spec §4 requires background jobs to be efficient and the addendum §13.3 defines block expiry as a scheduled job that must handle bulk operations.

## Evidence

- `SecurityAddendumJobWorker.cs` `ExpireBlocksAsync` iterates over subjects individually
- No use of EF Core `Include`/`ThenInclude` or batch update operations

## Expected Outcome

Block expiry should use a single batch query to:
1. Find all subjects with expired blocks in one query (`WHERE SoftBlockedUntilUtc < now OR FirewallBlockedUntilUtc < now`)
2. Update their states in a single batch operation via `ExecuteUpdateAsync`
3. Insert expiry events in bulk

## Fix Guidance

1. Use `db.SecuritySubjectStates.Where(s => s.SoftBlockedUntilUtc < now || s.FirewallBlockedUntilUtc < now)` to find all expired entries in one query.
2. Use `ExecuteUpdateAsync` to batch-clear the block state, or load them with related data via `Include` and update together.
3. Generate all expiry events in memory and insert with `AddRange` + single `SaveChanges`.
4. Consider setting a reasonable batch size limit (e.g., 1000) for very large expiry runs.

## Acceptance Criteria

- [ ] Block expiry uses a single query to find expired subjects
- [ ] Updates are batched (either `ExecuteUpdateAsync` or `UpdateRange` + single `SaveChanges`)
- [ ] Block expiry job runtime is not proportional to the number of expired blocks
- [ ] Database query count does not increase linearly with expired block count
