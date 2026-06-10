# H-043: PasskeyAuthService Uses credentialId.SequenceEqual — EF Core Cannot Translate

**Priority:** Low
**Conflict Type:** bad_implementation
**Spec Reference:** Main Spec §4, §7.8

**Status:** Not Started
**Branch:** 

## Description

In `PasskeyAuthService.CompleteLoginAsync()` (line 110-113), the credential lookup uses `SequenceEqual` on a byte array property:

```csharp
var stored = await db.PasskeyCredentials.SingleOrDefaultAsync(
    x => x.CredentialId.SequenceEqual(credentialId),
    cancellationToken)
```

Entity Framework Core cannot translate `SequenceEqual` on byte arrays into a SQL query. This forces EF Core to:
1. Load all passkey credential rows from the database
2. Perform the comparison in memory (client-side evaluation)

As the number of passkeys grows (spec §9 supports "multiple passkeys"), this becomes a full table scan on every login. While passkey counts are typically small (single-digit), this is still an unnecessary performance concern and violates the spec §4 requirement to "use raw SQL only for partitioning, advisory locks, and high-volume status/security inserts."

## Evidence

```csharp
// PasskeyAuthService.cs:110-113
var stored = await db.PasskeyCredentials.SingleOrDefaultAsync(
    x => x.CredentialId.SequenceEqual(credentialId),
    cancellationToken)
```

`PasskeyCredentialEntity.CredentialId` is defined as `public byte[] CredentialId { get; set; } = [];` which EF Core stores as a `bytea` column in PostgreSQL. While PostgreSQL can compare `bytea` columns, EF Core's LINQ provider does not generate the correct SQL for `SequenceEqual`.

## Expected Outcome

The credential lookup should use a query that EF Core can translate to SQL, such as:
1. Storing a hash of the credential ID and comparing hashes
2. Using a stored computed property that EF Core can translate
3. Using `EF.Functions` or raw SQL for the byte array comparison

## Fix Guidance

1. Add a `CredentialIdHash` computed property (SHA-256) stored alongside the credential and use it for lookup:
```csharp
public byte[] CredentialIdHash { get; set; } = [];
// Lookup:
var credentialIdHash = SHA256.HashData(credentialId);
var stored = await db.PasskeyCredentials.SingleOrDefaultAsync(
    x => x.CredentialIdHash.SequenceEqual(credentialIdHash), cancellationToken);
```

2. Alternatively, use the credential's GUID `Id` field for lookup since `PasskeyLoginResult` returns `CredentialId` (a `Guid`), and the credential is already looked up by GUID in most code paths.

## Acceptance Criteria

- [ ] Credential lookup by credential ID uses SQL-translatable comparison
- [ ] No client-side evaluation warning from EF Core in query logs
- [ ] Credential lookup is O(1) regardless of passkey count
- [ ] Registration uniqueness check also uses efficient comparison
