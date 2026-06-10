namespace Hashi.Core.Dns;

public enum DnsRecordType
{
    A,
    Aaaa,
    Cname,
    Mx,
    Txt,
    Ns,
    Soa,
    Other,
}

public enum DnsOwnershipKind
{
    Unknown,
    Imported,
    Managed,
    System,
    User,
}

public enum DnsChangeKind
{
    Create,
    Update,
    Delete,
    NoOp,
}

public sealed record DnsZone(string ProviderZoneId, string Name, int TtlDefault);

public sealed record DnsRecordSnapshot(
    string ProviderRecordId,
    string Name,
    DnsRecordType Type,
    string Value,
    int? Ttl,
    bool IsManagedByHashi);

public sealed record DnsPlanChange(
    DnsChangeKind Kind,
    string Name,
    DnsRecordType Type,
    string? CurrentValue,
    string? DesiredValue,
    int? Ttl,
    string RiskReason)
{
    public string? ProviderRecordId { get; init; }
}

public sealed record DnsSyncPlan(
    Guid PlanId,
    Guid ConnectionId,
    string ZoneName,
    IReadOnlyList<DnsPlanChange> Changes,
    bool RequiresConfirmation);

public static class DnsSafetyRules
{
    private static readonly HashSet<DnsRecordType> ProtectedTypes =
    [
        DnsRecordType.Ns,
        DnsRecordType.Soa,
    ];

    public static bool IsProtectedType(DnsRecordType type) => ProtectedTypes.Contains(type);

    public static bool CanDelete(DnsRecordSnapshot record)
        => record.IsManagedByHashi && !IsProtectedType(record.Type);

    public static bool CanModify(DnsRecordSnapshot record, DnsChangeKind kind)
    {
        if (IsProtectedType(record.Type))
        {
            return false;
        }

        return record.IsManagedByHashi && kind is (DnsChangeKind.Update or DnsChangeKind.NoOp);
    }

    public static DnsPlanChange? GuardChange(DnsPlanChange change)
    {
        if (change.Kind == DnsChangeKind.Delete && IsProtectedType(change.Type))
        {
            return change with
            {
                Kind = DnsChangeKind.NoOp,
                RiskReason = "NS/SOA records cannot be deleted.",
            };
        }

        if (change.Kind is DnsChangeKind.Update or DnsChangeKind.Delete && IsProtectedType(change.Type))
        {
            return change with
            {
                Kind = DnsChangeKind.NoOp,
                RiskReason = "NS/SOA records cannot be modified.",
            };
        }

        return null;
    }
}

public static class DnsRecordTypeMapping
{
    public static DnsRecordType Parse(string type) => type.ToUpperInvariant() switch
    {
        "A" => DnsRecordType.A,
        "AAAA" => DnsRecordType.Aaaa,
        "CNAME" => DnsRecordType.Cname,
        "MX" => DnsRecordType.Mx,
        "TXT" => DnsRecordType.Txt,
        "NS" => DnsRecordType.Ns,
        "SOA" => DnsRecordType.Soa,
        _ => DnsRecordType.Other,
    };

    public static string ToApiName(DnsRecordType type) => type switch
    {
        DnsRecordType.A => "A",
        DnsRecordType.Aaaa => "AAAA",
        DnsRecordType.Cname => "CNAME",
        DnsRecordType.Mx => "MX",
        DnsRecordType.Txt => "TXT",
        DnsRecordType.Ns => "NS",
        DnsRecordType.Soa => "SOA",
        _ => "OTHER",
    };
}
