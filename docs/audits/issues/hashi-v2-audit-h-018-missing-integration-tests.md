# H-018: Missing Integration Tests for Key Flows

**Priority:** Medium
**Conflict Type:** missing_implementation
**Spec Reference:** Main Spec §31 (Test Strategy - Integration tests)

**Status:** Not Started
**Branch:** 

## Description

The spec requires integration tests for:
```
Integration tests:
- PostgreSQL migrations.
- Sync plan persistence.
- Hetzner fake API.
- AdGuard fake API.
- SSH/SFTP test container.
- Traefik config validation container.
- OIDC fake provider.
- SMTP fake server.
```

The actual integration test suite (10 test files) covers:
- ✅ PostgreSQL migrations (`SetupPersistenceTests.cs`, `MonitorSamplePartitionTests.cs`)
- ✅ Hetzner fake API (`HetznerDnsPlanApplyTests.cs` with `HetznerDnsFakeHandler.cs`)
- ✅ OIDC fake provider (`EdgeAuthOidcTests.cs`)
- ✅ Setup flow (`SetupFlowIntegrationTests.cs`)
- ✅ End-to-end platform (`EndToEndPlatformTests.cs`)

**Missing:**
- ❌ AdGuard fake API integration tests
- ❌ SSH/SFTP test container tests (only `SshRemoteExecutorTests.cs` exists, may use mocks)
- ❌ Traefik config validation container tests
- ❌ SMTP fake server tests
- ❌ Sync plan persistence comprehensive tests

## Evidence

Integration test files:
```csharp
// Hashi.IntegrationTests/
EdgeAuthOidcTests.cs           // OIDC integration
EndToEndPlatformTests.cs       // Platform integration
HetznerDnsPlanApplyTests.cs    // DNS integration
MonitorSamplePartitionTests.cs // Monitoring integration
SetupFlowIntegrationTests.cs   // Setup integration
SetupPersistenceTests.cs       // Persistence integration
SshRemoteExecutorTests.cs      // SSH integration
VaultStatusTests.cs            // Vault integration
```

Missing test containers for:
- AdGuard Home API
- Traefik config validation
- SMTP server

## Expected Outcome

- All spec-required integration test areas have tests
- External service integrations use fakes/containers
- Database migrations are tested

## Fix Guidance

1. Add AdGuard Home fake API integration tests
2. Add Traefik config validation container tests
3. Add SMTP fake server tests
4. Verify SSH tests use proper test containers

## Acceptance Criteria

- [ ] AdGuard integration tests exist
- [ ] Traefik config validation tests exist
- [ ] SMTP integration tests exist
- [ ] All external services have fake/test implementations
