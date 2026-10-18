# AUD-116: Concurrent security requests lose counters or create duplicate buckets

Priority: P1

Category: Reliability / concurrent security ingestion

Status: open; confirmed by manual source trace.

## Finding

Subject/state/abuse/request-bucket creation uses read-then-insert without conflict recovery. Concurrent first requests can hit unique constraints and fail. SecurityRequestBucket uniqueness additionally includes nullable CountryCode/RegionCode/Asn; PostgreSQL treats nulls as distinct by default, so concurrent common no-geo inserts can both succeed. Later SingleOrDefaultAsync then throws for that same bucket until the time window changes or rows are repaired. Existing bucket counters use tracked read/increment/save with no concurrency token or atomic update, losing concurrent increments. This affects live forward-auth, not just analytics.

## Evidence

src/Hashi.Infrastructure/Platform/SecuritySubjectService.cs:19-65; SecurityIngestionService.cs:37-42,92-137,267-316; Persistence/HashiDbContext.cs:797-831. No NULLS NOT DISTINCT configuration found. PostgreSQL semantics: https://www.postgresql.org/docs/current/indexes-unique.html .

## Expected outcome and acceptance criteria

Use atomic upserts/updates and a null-safe canonical bucket key, deduplicate old rows before adding the correct uniqueness constraint, and recover concurrent subject creation. Independent parallel requests must preserve exact counters and never poison future lookups.
