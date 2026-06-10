# H-017: Missing Unit Tests for Several Core Areas

**Priority:** Medium
**Conflict Type:** missing_implementation
**Spec Reference:** Main Spec §31 (Test Strategy - Unit tests)

**Status:** Not Started
**Branch:** 

## Description

The spec requires unit tests for:
```
Unit tests:
- Domain validation.
- DNS diffing.
- Traefik render output.
- Firewall render output.
- Rule evaluation.
- Vault wrapping/unwrapping.
- Status rollups.
- Abuse scoring.
```

The actual unit test suite (49 test files) covers most areas but has gaps:

**Covered:**
- ✅ Domain validation (`DnsRecordGeneratorTests.cs`)
- ✅ DNS diffing (`DnsRecordServiceTests.cs`)
- ✅ Traefik render output (`TraefikSyncSafetyTests.cs`)
- ✅ Firewall render output (`DnsRecordGeneratorTests.cs` includes FirewallScriptRendererTests)
- ✅ Vault wrapping/unwrapping (`VaultSecretBoundaryTests.cs`)
- ✅ Abuse scoring (`SecurityDecisionServiceTests.cs`, `SecurityIngestionServiceTests.cs`)

**Potentially Missing or Incomplete:**
- ❓ Rule evaluation (partial - `SecurityDecisionServiceTests.cs` covers some)
- ❓ Status rollups (`MonitoringServiceTests.cs` exists but may not cover all rollup scenarios)
- ❓ DNS diffing edge cases
- ❓ Traefik render output comprehensive testing

## Evidence

Unit test files found:
- `SecurityDecisionServiceTests.cs` - Covers rule evaluation
- `MonitoringServiceTests.cs` - Covers status monitoring
- `DnsRecordGeneratorTests.cs` - Covers DNS record generation
- `TraefikSyncSafetyTests.cs` - Covers Traefik sync safety
- `FirewallApplySafetyTests.cs` - Covers firewall apply safety

However, specific test coverage for:
- Complete rule evaluation (IP, CIDR, path, country, region, ASN matching)
- All status rollup scenarios (1m, 5m, 1h rollups)
- DNS diffing edge cases (NS/SOA protection, unowned record handling)

## Expected Outcome

- All spec-required unit test areas have dedicated tests
- Edge cases are covered
- Safety requirements are tested

## Fix Guidance

1. Verify rule evaluation tests cover all match types (IP, CIDR, path, country, region, ASN)
2. Add comprehensive DNS diffing tests for edge cases
3. Add status rollup tests for all intervals
4. Add Traefik render output tests for all resource types

## Acceptance Criteria

- [ ] Rule evaluation tests cover all match types
- [ ] DNS diffing tests cover NS/SOA protection
- [ ] Status rollup tests cover 1m, 5m, 1h intervals
- [ ] Traefik render tests cover HTTP, TCP, UDP resources
