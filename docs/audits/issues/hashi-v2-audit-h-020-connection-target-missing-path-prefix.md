# H-020: ConnectionTargetEntity Missing path_prefix Field

**Priority:** Medium
**Conflict Type:** missing_implementation
**Spec Reference:** Addendum §9.2

**Status:** Not Started
**Branch:** 

## Description

The Addendum specifies the connection target model with these fields:
```
Fields:
- target_mode
- static_host
- static_ip
- pulse_agent_id
- pulse_ip_mode
- private_candidate_selector
- port
- scheme
- path_prefix
- tls_validation_mode
- expected_hostname
- resolved_ip_snapshot
- last_resolved_at_utc
- resolution_status
- resolution_error
```

The `ConnectionTargetEntity` in `ExtendedPlatformEntities.cs` includes most fields but has `ExpectedTlsHostname` instead of `expected_hostname`. This is a naming deviation but the field exists.

However, `path_prefix` is present in the entity:
```csharp
public string? PathPrefix { get; set; }
```

This finding is a false positive - the field exists. Let me verify other fields.

Checking the entity:
- `TargetMode` ✅
- `StaticHost` ✅
- `StaticIp` ✅
- `PulseAgentId` ✅
- `PulseIpMode` ✅
- `PrivateCandidateSelector` ✅
- `Port` ✅
- `Scheme` ✅
- `PathPrefix` ✅
- `TlsValidationMode` ✅
- `ExpectedTlsHostname` ✅ (named differently but exists)
- `ResolvedIpSnapshot` ✅
- `LastResolvedAtUtc` ✅
- `Status` ✅ (maps to `resolution_status`)
- `LastError` ✅ (maps to `resolution_error`)

All fields are present. This finding is a false positive.

## Expected Outcome

- All Addendum §9.2 fields are present in the entity

## Fix Guidance

No changes needed - all fields are present.

## Acceptance Criteria

- [x] All target model fields are present (implemented)
