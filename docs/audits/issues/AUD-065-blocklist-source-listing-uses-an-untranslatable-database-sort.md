# AUD-065: Blocklist source listing uses an untranslatable database sort

Priority: P1

Category: Security feature / blocklist management

Status: open; confirmed by manual source tracing.

## Finding

ListAsync orders an EF IQueryable by RecommendedMetadata.IsRecommended(x.MetadataJson), an application method that deserializes arbitrary JSON. It is outside the top-level projection and has no database function mapping in this implementation. A relational provider cannot translate it, so the source list throws instead of displaying recommended/custom feeds. This affects the PostgreSQL deployment even if in-memory tests accept it.

## Evidence

src/Hashi.Infrastructure/Platform/BlocklistSourceManagementService.cs:24-31,1292-1313

## Expected outcome

Materialize before this small metadata sort or store/query a translatable recommended field.

## Acceptance criteria

GET source list succeeds on PostgreSQL with recommended and custom sources and orders them as intended.

No live provider state was modified to establish this finding.

Primary reference: [Microsoft behavior documentation](https://learn.microsoft.com/en-us/ef/core/querying/client-eval).
