# A04 - DNS ownership safety can modify unowned provider records

Priority: Critical

Spec conflicts: non-negotiable rules 1, 2, 3, 7, and 20; DNS ownership rules in section 15.

## Problem

Provider records from Hetzner are marked `IsManagedByHashi: false`, but the planner still updates any provider record that matches a desired `name|type` key. That can modify a user-owned provider record simply because Hashi wants a record with the same name and type.

Deletion handling is safer because it skips current records where `!existing.IsManagedByHashi`, but update handling does not use the same ownership gate. The `CanDelete` helper is also inverted: it returns true for unowned records or protected NS/SOA records.

Import preview hides NS/SOA records completely instead of displaying them as protected/unselectable, which weakens the auditability of what Hashi saw at the provider.

## Evidence

- `src/Hashi.Infrastructure/Providers/Dns/HetznerDnsProvider.cs:31-37` marks listed provider records as `IsManagedByHashi: false`.
- `src/Hashi.Core/Dns/DnsPlanner.cs:30-40` creates an update for any matching current record when value or TTL differs.
- `src/Hashi.Core/Dns/DnsPlanner.cs:55-59` skips unowned current records only in the delete pass.
- `src/Hashi.Core/Dns/DnsModels.cs:68-69` implements `CanDelete` as `!record.IsManagedByHashi || IsProtectedType(record.Type)`.
- `src/Hashi.Infrastructure/Dns/DnsConnectionService.cs:146-148` filters protected records out of import preview.

## Expected outcome

Hashi must never modify provider records unless they were created by Hashi, imported by the user, or explicitly assigned to Hashi. NS/SOA records must never be modified or deleted. Provider reads should produce a visible ownership decision, not blind updates.

## Fix guidance

Introduce explicit ownership lookup when building provider snapshots. For records matching desired state but not owned, plan a conflict/manual action instead of update. Correct `CanDelete`. Show NS/SOA records in previews as protected and unselectable. Add tests for "desired record collides with unowned provider record" and "NS/SOA appears protected in preview."

## Acceptance criteria

- Unowned provider records are never updated or deleted by sync.
- Matching desired names produce a blocked/conflict plan item until the user imports or assigns ownership.
- `CanDelete` returns true only for managed, non-protected records.
- Import preview includes protected NS/SOA as non-actionable evidence.
