# H-021: ResourceRuleEntity Missing Match Fields for Country/Region/ASN

**Priority:** Medium
**Conflict Type:** missing_implementation
**Spec Reference:** Main Spec §6 (Resource Rule Model)

## Description

The spec defines resource rules with match types:
```
Match types:
- IP.
- IP range/CIDR.
- Path.
- Country.
- Region.
- ASN.
```

The `ResourceRuleEntity` in `ExtendedPlatformEntities.cs` has:
```csharp
public string MatchType { get; set; } = "path";
public string MatchValue { get; set; } = "/";
```

This uses a generic `MatchType` string and `MatchValue` string approach. The spec's match types (IP, CIDR, path, country, region, ASN) are represented as string values in `MatchType`, which is a valid approach.

However, the spec mentions that "Country, region, and ASN matching require a GeoIP database. If unavailable, those rules become invalid with a clear validation error and cannot be enabled." The entity model does not have a field to track whether GeoIP data is available for validation.

## Evidence

```csharp
// ExtendedPlatformEntities.cs
public sealed class ResourceRuleEntity
{
    public Guid Id { get; set; } = Guid.NewGuid();
    public Guid ResourceId { get; set; }
    public bool Enabled { get; set; } = true;
    public int Priority { get; set; }
    public string Action { get; set; } = "pass_to_auth";
    public string MatchType { get; set; } = "path";
    public string MatchValue { get; set; } = "/";
}
```

The `MatchType` field can hold "ip", "cidr", "path", "country", "region", "asn" as string values. This is a valid implementation approach. The validation for GeoIP availability would be in the service layer, not the entity.

## Expected Outcome

- All match types are supported
- GeoIP-dependent rules are validated
- Rules are evaluated correctly

## Fix Guidance

The entity model is correct. Validation for GeoIP availability should be in the service/validator layer. No entity changes needed.

## Acceptance Criteria

- [x] MatchType field supports all spec match types (implemented)
- [ ] GeoIP validation exists in service layer (needs verification)
- [ ] Rules are validated before enabling
