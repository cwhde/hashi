# H-023: MonitorEndpointEntity Missing Group/CheckType Fields

**Priority:** Medium
**Conflict Type:** missing_implementation
**Spec Reference:** Main Spec §18.1, §18.2

**Status:** In Progress
**Branch:** h/monitoring-dns-firewall
**Branch:** 

## Description

The spec defines monitor endpoints with these fields:
```
Endpoints are created from:
- Resources.
- Manual DNS entries with monitoring enabled.
- Linux firewall hosts.
- Traefik connections.
- AdGuard Home connections.
- Hashi itself.
- Optional user-created monitor endpoints.
```

And check types:
```
Supported check types:
- HTTP.
- HTTPS.
- H2C where practical.
- TCP.
- UDP basic response checks where configured.
- DNS.
- ICMP when container capabilities allow it.
- TLS certificate expiry.
- Push-based Pulse health.
```

The `MonitorEndpointEntity` has:
```csharp
public string CheckType { get; set; } = "https";
```

The entity does not have a `Group` field for grouping endpoints by host, firewall host, status, or resource type as specified in §18.5:
```
Group by host, Linux firewall host, status, or resource type.
```

The grouping is likely handled in the UI layer (frontend) rather than the entity model. This is acceptable since grouping is a presentation concern.

## Evidence

```csharp
public sealed class MonitorEndpointEntity
{
    public Guid Id { get; set; } = Guid.NewGuid();
    public string Name { get; set; } = string.Empty;
    public string Url { get; set; } = string.Empty;
    public string CheckType { get; set; } = "https";
    public bool Enabled { get; set; } = true;
    public bool PublicStatusEnabled { get; set; }
    public string Status { get; set; } = "unknown";
    public Guid? ResourceId { get; set; }
    public Guid? DnsRecordId { get; set; }
    public DateTimeOffset? LastCheckedAtUtc { get; set; }
    public int? LastLatencyMs { get; set; }
}
```

Missing from entity:
- No `Group` field (grouping is UI concern, acceptable)
- No `CheckIntervalSeconds` field (may be in AppSettingsEntity)
- No `TimeoutSeconds` field (may be in AppSettingsEntity)
- No `AllowedStatusCodes` field (may be in settings)

## Expected Outcome

- Monitor endpoints have all necessary fields
- Grouping is handled appropriately (UI or entity)
- Check configuration is complete

## Fix Guidance

The entity model is functional. The missing fields (group, interval, timeout) are likely handled through:
- `AppSettingsEntity` for global defaults
- Frontend for grouping logic
- Service layer for check execution

No entity changes strictly required, but adding a `Group` field could simplify queries.

## Acceptance Criteria

- [x] CheckType field exists (implemented)
- [x] ResourceId linkage exists (implemented)
- [ ] Verify grouping logic exists in UI or service layer
- [ ] Verify check interval/timeout configuration exists
