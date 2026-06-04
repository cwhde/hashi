# TASK-01: Data Model and Contract Foundation

## Goal

Add the persistent schema, EF entities, indexes, DTOs, and OpenAPI contract surface needed by the addendum without changing live behavior yet.

This is the foundation for all other addendum tasks.

## Spec Context

- Original spec sections: 8, 9, 11, 13, 15, 16, 17, 25, 26, 27, 31.
- Addendum sections: 5.5, 6.1, 8.6, 9.2, 10.4, 12, 16, 18, 19 Phase A.

## Current Code Anchors

- Entities: `src/Hashi.Infrastructure/Persistence/Entities/CoreEntities.cs`
- Existing platform/security entities: `src/Hashi.Infrastructure/Persistence/Entities/ExtendedPlatformEntities.cs`
- Resource/Pulse entities: `src/Hashi.Infrastructure/Persistence/Entities/PlatformEntities.cs`
- DbContext mappings: `src/Hashi.Infrastructure/Persistence/HashiDbContext.cs`
- Contracts: `src/Hashi.Contracts/Api/PlatformContracts.cs`, `src/Hashi.Contracts/Api/ConnectionContracts.cs`
- Migration discovery test: `tests/Hashi.UnitTests/PersistenceMigrationDiscoveryTests.cs`
- Existing security tests: `tests/Hashi.UnitTests/EdgeAuthServiceTests.cs`, `tests/Hashi.UnitTests/SecurityIngestionServiceTests.cs`

## Required Model Work

Add normalized subject tables:

- `security_subjects`
- `security_subject_states`
- `manual_security_entries`

Extend or replace the current thin security event/bucket shape:

- Extend `security_events` with addendum fields while preserving existing dashboard reads.
- Extend `security_request_buckets` from IP-only shape toward normalized subject/root-domain/resource dimensions.
- Decide whether `abuse_buckets` remains as a compatibility table or becomes a temporary migration source for `security_subject_states`.

Add ban/escalation settings:

- Add a safe policy model for constant, linear, exponential, capped exponential, and permanent-after-N policies.
- Store global defaults in `AppSettingsEntity` JSON fields or dedicated settings tables.
- Leave room for resource security profile overrides.

Add blocklist source tables:

- `blocklist_sources`
- `blocklist_entries` extensions for source ID, normalized value, subject type, enabled, enforcement mode, metadata.
- `blocklist_fetch_runs`

Do not lose compatibility with the existing `blocklist_entries` fields until all callers are migrated.

Add CAPTCHA settings:

- Cap public challenge base URL.
- Site key.
- Secret key secret reference.
- Verification timeout.
- Optional Cap admin resource ID/domain.
- Public challenge system resource ID/domain.
- Challenge reset/decay settings.

Add connection target model:

- `target_mode`: `static_host`, `static_ip`, `pulse_agent`.
- Static host/IP fields.
- Pulse agent ID, IP mode, private candidate selector.
- Port, scheme, path prefix.
- TLS validation mode and expected hostname.
- Resolved IP snapshot, last resolved time, status, error.

Recommended shape: a reusable `connection_targets` table referenced by connection-like integrations, or an owned-value pattern with clear migration path. At minimum, AdGuard must be able to use it in task 06.

Add internal agent DNS settings:

- Enable flag.
- Domain default `hashi.home.arpa`.
- Per-agent enabled flag/name override/IP mode.
- Stale handling flag, default keep last rewrite.

## Deliverables

- EF entity classes and DbSets.
- EF model configuration and indexes matching addendum lookup needs.
- New migration and updated `HashiDbContextModelSnapshot`.
- Contract records for subject search/detail, manual entries, blocklists, CAPTCHA settings, connection targets, resolved Pulse targets, and internal agent DNS.
- OpenAPI and generated frontend type artifacts regenerated.
- Compatibility notes in migration comments or tests for current tables.

## Tests

- Migration discovery and model snapshot tests.
- Unit tests for default values, especially manual allow bypass flags:
  - `bypassBlocking = true`
  - `bypassAdaptiveEscalation = true`
  - `bypassRateLimit = false`
  - `bypassChallenge = false`
  - `bypassSso = false`
- Tests that block entries cannot have bypass flags.
- Tests that normalized subject unique index behavior is deterministic.
- Tests that existing `blocklist_entries`, `abuse_buckets`, `security_events`, and `security_request_buckets` data can survive migration.

## Acceptance

- `dotnet test Hashi.slnx /p:SkipFrontendBuild=true` passes.
- OpenAPI export and frontend type generation are clean.
- Existing security dashboard and forward-auth tests still pass before behavior changes land.
