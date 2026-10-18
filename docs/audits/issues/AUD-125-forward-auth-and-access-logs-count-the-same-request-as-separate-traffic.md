# AUD-125: Forward auth and access logs count the same request as separate traffic

Priority: P2

Category: Wrong implementation / security event accounting

Status: open; confirmed by manual source trace.

## Finding

The live authorization endpoint records every decision and AccessLogIngestWorker later records the corresponding completed proxy request. Both ingestion methods increment SecurityRequestBuckets and create access/security events; RequestId is stored but never checked for correlation/deduplication. Forward-auth uses a synthetic status and instance name, so analytics can present the authorization attempt and completed request as two independent traffic events, with divergent status categories. Source-specific telemetry can be useful, but it must not silently inflate product request totals or thresholds.

## Evidence

src/Hashi.Api/Features/EdgeAuth/EdgeAuthEndpoints.cs:81-92; src/Hashi.Infrastructure/Platform/SecurityIngestionService.cs:22-180,232-349; AccessLogIngestWorker.cs:257-275. AUD-023 separately covers replay from log cursor/rotation behavior.

## Expected outcome and acceptance criteria

Define one canonical traffic request record and correlate decision/response phases by a stable propagated ID; retain phase-specific events without double counting traffic or offenses. One request seen through both ingestion paths must contribute once to product request totals.
