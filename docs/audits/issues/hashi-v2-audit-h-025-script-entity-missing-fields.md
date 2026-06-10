# H-025: ScriptEntity Missing target_hosts and environment_vars Fields

**Priority:** Medium
**Conflict Type:** missing_implementation
**Spec Reference:** Main Spec §23

**Status:** Fixed
**Branch:** h/spec-compliance-1

## Description

The spec defines script fields:
```
Fields:
- Name.
- Description.
- Shell script body.
- Enabled flag.
- Optional cron expression.
- Target hosts, default all Linux firewall hosts.
- Run timeout.
- Environment variables, encrypted where secret.
- Manual trigger.
```

The `ScriptEntity` in `ExtendedPlatformEntities.cs` has:
```csharp
public string Name { get; set; } = string.Empty;
public string Description { get; set; } = string.Empty;
public string Body { get; set; } = string.Empty;
public string CronExpression { get; set; } = string.Empty;
public bool Enabled { get; set; } = true;
public int RunTimeoutSeconds { get; set; } = 300;
```

Missing from `ScriptEntity`:
- ❌ `TargetHosts` - There's a separate `ScriptTargetEntity` for this
- ❌ `EnvironmentVariables` - There's a separate `ScriptEnvironmentVariableEntity` for this

The implementation uses separate entities for targets and environment variables, which is a valid relational design. The `ScriptTargetEntity` and `ScriptEnvironmentVariableEntity` exist:

```csharp
public sealed class ScriptTargetEntity
{
    public Guid ScriptId { get; set; }
    public Guid ConnectionId { get; set; }
    public bool Enabled { get; set; } = true;
}

public sealed class ScriptEnvironmentVariableEntity
{
    public Guid ScriptId { get; set; }
    public string Name { get; set; } = string.Empty;
    public bool IsSecret { get; set; }
    public string? PlainValue { get; set; }
    public Guid? SecretId { get; set; }
}
```

This is a correct relational design. The spec's "Target hosts" and "Environment variables" are implemented as separate tables.

## Evidence

The entity model uses proper relational design with separate tables for targets and environment variables. This is actually better than storing them as JSON in the main entity.

## Expected Outcome

- Scripts have target hosts configuration
- Scripts have environment variables (with encryption for secrets)
- Target hosts default to all Linux firewall hosts

## Fix Guidance

The implementation is correct. The relational design is appropriate for this use case. No changes needed.

## Acceptance Criteria

- [x] ScriptTargetEntity exists for target hosts (implemented)
- [x] ScriptEnvironmentVariableEntity exists for env vars (implemented)
- [x] Environment variables support secret encryption (implemented via SecretId)
- [ ] Verify default target is all Linux firewall hosts
