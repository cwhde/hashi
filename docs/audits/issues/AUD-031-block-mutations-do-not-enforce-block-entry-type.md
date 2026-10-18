# AUD-031: Block mutation endpoints accept manual allow entry IDs

Priority: P3 — API operation can corrupt security state for the wrong entry kind

Category: Wrong endpoint behavior / security-state integrity

Spec reference: implementation addendum §5.3 separates manual allow and manual block actions; block actions must operate on block entries.

## Finding

The block update and expire operations do not consistently restrict their database lookup to `EntryType == block`. `UpdateBlockAsync` delegates to the generic manual-entry updater, which accepts any ID, then may apply soft/firewall block state to that entry's subject. `ExpireBlockAsync` delegates to the generic expire operation and then clears block state, also accepting an allow entry ID. Other block-specific operations (`ExtendBlockAsync`, `ShortenBlockAsync`, `MakePermanentAsync`, and preview) correctly filter for block entries. A caller using the documented block endpoint with a valid allow-entry ID therefore receives a successful mutation and can create inconsistent subject state.

## Evidence

- `src/Hashi.Infrastructure/Platform/SecuritySubjectOperationsService.cs`: `UpdateBlockAsync` calls `UpdateManualEntryAsync(id, ...)`, which looks up by ID only, and then applies block state when `FirewallEnforced` is provided.
- `ExpireBlockAsync` calls `ExpireManualEntryAsync`, whose lookup is also by ID only, before clearing block state.
- `ExtendBlockAsync`, `ShortenBlockAsync`, and `MakePermanentAsync` explicitly include the block entry-type predicate.
- `src/Hashi.Api/Features/Security/SecuritySubjectEndpoints.cs`: these operations are exposed under `/api/security/blocks/{id}`.

## Expected outcome

All block-specific service operations should resolve only block entries. An allow ID should behave as not found and must not mutate the allow entry or subject block state.

## Acceptance criteria

- Update/expire/extend/shorten/make-permanent/firewall-preview routes reject an allow-entry ID without changing either the entry or subject state.
- Existing block IDs continue to support the advertised mutations.
- Generic manual-entry routes retain their intentional allow/block behavior.
