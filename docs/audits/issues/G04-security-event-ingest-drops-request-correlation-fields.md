# G04 - Security event ingest drops request correlation fields

Priority: Medium

Spec conflicts: addendum sections 2.4 and 14

## Problem

The addendum requires security events to retain request correlation fields, including `user_agent_hash` and `request_id`. The entity has columns for those values, but the ingest contracts do not accept them and the ingestion paths do not populate them.

This makes the data model look compliant while request-level investigation still loses important correlation data. Forward-auth decisions, WAF events, and Traefik access-log ingestion cannot preserve request IDs or hashed user agents in `security_events`.

## Evidence

- `docs/implementation-spec/hashi-v2-implementation-spec-addendum.md:291` through `docs/implementation-spec/hashi-v2-implementation-spec-addendum.md:313` lists required `security_events` fields, including `user_agent_hash` and `request_id`.
- `src/Hashi.Infrastructure/Persistence/Entities/ExtendedPlatformEntities.cs:324` and `src/Hashi.Infrastructure/Persistence/Entities/ExtendedPlatformEntities.cs:326` include `UserAgentHash` and `RequestId` on `SecurityEventEntity`.
- `src/Hashi.Contracts/Api/PlatformContracts.cs:1039` through `src/Hashi.Contracts/Api/PlatformContracts.cs:1048` defines `ForwardAuthDecisionIngestRequest` without request ID or user-agent input.
- `src/Hashi.Contracts/Api/PlatformContracts.cs:1050` through `src/Hashi.Contracts/Api/PlatformContracts.cs:1054` defines `WafEventIngestRequest` without request ID or user-agent input.
- `src/Hashi.Contracts/Api/PlatformContracts.cs:1158` through `src/Hashi.Contracts/Api/PlatformContracts.cs:1169` defines `AccessLogIngestRequest` without request ID or user-agent input.
- `src/Hashi.Infrastructure/Platform/SecurityIngestionService.cs:147` through `src/Hashi.Infrastructure/Platform/SecurityIngestionService.cs:163`, `src/Hashi.Infrastructure/Platform/SecurityIngestionService.cs:322` through `src/Hashi.Infrastructure/Platform/SecurityIngestionService.cs:338`, and `src/Hashi.Infrastructure/Platform/SecurityIngestionService.cs:349` through `src/Hashi.Infrastructure/Platform/SecurityIngestionService.cs:356` create security events without setting either field.
- `src/Hashi.Infrastructure/Platform/AccessLogIngestWorker.cs:215` through `src/Hashi.Infrastructure/Platform/AccessLogIngestWorker.cs:252` parses Traefik JSON into `AccessLogIngestRequest` without extracting a request ID or user agent.

## Expected outcome

Security event ingestion should accept and persist request IDs and privacy-preserving user-agent hashes wherever the source provides them.

## Fix guidance

Add optional request ID and user-agent/user-agent-hash fields to the ingest contracts. Hash user agents before storage if raw user-agent input is accepted. Populate `SecurityEventEntity.RequestId` and `SecurityEventEntity.UserAgentHash` in access-log, forward-auth, and WAF ingestion paths, and expose the fields in event responses where appropriate.

## Acceptance criteria

- Forward-auth, WAF, and access-log ingest can provide request ID and user-agent information.
- Raw user-agent values are not stored when the spec expects a hash.
- `security_events.request_id` and `security_events.user_agent_hash` are populated in representative ingest tests.
- Security event responses include request correlation data needed for incident review.
