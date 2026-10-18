# AUD-117: Long valid request paths fail forward-auth persistence

Priority: P2

Category: Reliability / request-derived persistence

Status: open; confirmed by manual source trace.

## Finding

EdgeAuthEndpoints passes the complete forwarded URI as PathPrefix into IngestForwardAuthDecisionAsync. NormalizePathPrefix accepts a supplied prefix without length bounding, but the database column is varchar256. An otherwise valid protected request with a longer URI causes SaveChanges failure after the authorization decision; ingestion is outside the endpoint decision exception guard, so the client receives an error instead of its authorization result. Other request-derived dimensions also need deliberate bounds. Do not truncate silently in a way that merges security identities.

## Evidence

src/Hashi.Api/Features/EdgeAuth/EdgeAuthEndpoints.cs:57-92; src/Hashi.Infrastructure/Platform/SecurityIngestionService.cs:263,281-295,1051-1065; Persistence/HashiDbContext.cs:814.

## Expected outcome and acceptance criteria

Store a bounded normalized path category or use a suitable representation for full paths, validate all untrusted dimensions and isolate telemetry failures from the authorization response while preserving fail-closed enforcement. A URI over256 characters must produce the intended allow/challenge/deny response.
