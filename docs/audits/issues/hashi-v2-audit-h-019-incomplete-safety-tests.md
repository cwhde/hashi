# H-019: Incomplete Test Coverage for Safety Requirements

**Priority:** Medium
**Conflict Type:** missing_implementation
**Spec Reference:** Main Spec §31 (Test Strategy - Safety tests)

**Status:** Fixed
**Branch:** h/tests

## Description

The spec requires specific safety tests:
```
Safety tests:
- NS/SOA deletion impossible.
- Unowned records untouched.
- Identical Traefik file not rewritten.
- Firewall generated chains do not flush unrelated rules.
- NetBird-created rules are preserved.
- NetBird interface access survives Hashi firewall apply.
- Pulse target IPs matching managed hosts produce CNAMEs, not A records.
- Secret redaction in logs and API responses.
- Passive sync runs without an active web session.
- High-risk passive sync plans wait for user approval.
```

**Covered:**
- ✅ NS/SOA deletion impossible (`DnsSafetyTests.cs`)
- ✅ Unowned records untouched (`DnsSafetyTests.cs`)
- ✅ Identical Traefik file not rewritten (`TraefikSyncSafetyTests.cs`)
- ✅ Firewall generated chains do not flush unrelated rules (`FirewallApplySafetyTests.cs`)
- ✅ Pulse target IPs matching managed hosts produce CNAMEs (`DnsRecordGeneratorTests.cs`)
- ✅ Secret redaction (`VaultSecretBoundaryTests.cs`)
- ✅ Passive sync safety (`PassiveSyncSafetyTests.cs`)

**Potentially Missing:**
- ❓ NetBird-created rules preserved (may be in `FirewallApplySafetyTests.cs`)
- ❓ NetBird interface access survives firewall apply
- ❓ High-risk passive sync plans wait for approval

## Evidence

Safety test files found:
- `DnsSafetyTests.cs` - Tests NS/SOA protection and unowned records
- `TraefikSyncSafetyTests.cs` - Tests identical file detection
- `FirewallApplySafetyTests.cs` - Tests firewall chain isolation
- `DnsRecordGeneratorTests.cs` - Tests Pulse IP matching
- `VaultSecretBoundaryTests.cs` - Tests secret redaction
- `PassiveSyncSafetyTests.cs` - Tests passive sync behavior

Missing explicit tests for:
1. NetBird-created rules preservation after Hashi firewall apply
2. NetBird interface access survival
3. High-risk passive sync plan approval flow

## Expected Outcome

- All safety requirements have dedicated tests
- Edge cases in safety behavior are covered
- Destructive operations are prevented by tests

## Fix Guidance

1. Add explicit tests for NetBird rule preservation
2. Add tests for NetBird interface access after firewall apply
3. Add tests for high-risk passive sync plan approval
4. Document any safety requirements that cannot be automated

## Acceptance Criteria

- [ ] NetBird rule preservation test exists
- [ ] NetBird interface access test exists
- [ ] High-risk passive sync approval test exists
- [ ] All safety requirements have corresponding tests
