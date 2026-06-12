using Hashi.Contracts.Api;

namespace Hashi.Infrastructure.Persistence.Entities;

public sealed class ConnectionEntity
{
    public Guid Id { get; set; } = Guid.NewGuid();

    public string Name { get; set; } = string.Empty;

    public string Type { get; set; } = string.Empty;

    public bool Enabled { get; set; } = true;

    public string HealthState { get; set; } = ConnectionHealthStateNames.Unknown;

    public string? LastValidationMessage { get; set; }

    public DateTimeOffset? LastValidatedAtUtc { get; set; }

    public Guid? SecretId { get; set; }

    public string SettingsJson { get; set; } = "{}";

    public string DeletionPolicy { get; set; } = ConnectionDeletionPolicyNames.Optional;

    public DateTimeOffset CreatedAtUtc { get; set; } = DateTimeOffset.UtcNow;

    public DateTimeOffset UpdatedAtUtc { get; set; } = DateTimeOffset.UtcNow;
}

public sealed class ConnectionHealthEntity
{
    public Guid Id { get; set; } = Guid.NewGuid();

    public Guid ConnectionId { get; set; }

    public ConnectionEntity Connection { get; set; } = null!;

    public string State { get; set; } = ConnectionHealthStateNames.Unknown;

    public string CheckKind { get; set; } = "validation";

    public int? LatencyMs { get; set; }

    public string? Message { get; set; }

    public string? DetailsJson { get; set; }

    public DateTimeOffset CheckedAtUtc { get; set; } = DateTimeOffset.UtcNow;
}

public sealed class DnsZoneEntity
{
    public Guid Id { get; set; } = Guid.NewGuid();

    public Guid ConnectionId { get; set; }

    public ConnectionEntity Connection { get; set; } = null!;

    public string ProviderZoneId { get; set; } = string.Empty;

    public string Name { get; set; } = string.Empty;

    public int DefaultTtl { get; set; } = 3600;

    public DateTimeOffset CreatedAtUtc { get; set; } = DateTimeOffset.UtcNow;
}

public sealed class DnsRecordEntity
{
    public Guid Id { get; set; } = Guid.NewGuid();

    public Guid ZoneId { get; set; }

    public DnsZoneEntity Zone { get; set; } = null!;

    public string ProviderRecordId { get; set; } = string.Empty;

    public string Name { get; set; } = string.Empty;

    public string Type { get; set; } = string.Empty;

    public string Value { get; set; } = string.Empty;

    public int? Ttl { get; set; }

    public string Ownership { get; set; } = DnsOwnershipNames.Unknown;

    public bool Enabled { get; set; } = true;

    public bool DashboardEnabled { get; set; }

    public string? DashboardDisplayName { get; set; }

    public bool MonitoringEnabled { get; set; }

    public string? MonitoringDisplayName { get; set; }

    public DateTimeOffset CreatedAtUtc { get; set; } = DateTimeOffset.UtcNow;

    public DateTimeOffset UpdatedAtUtc { get; set; } = DateTimeOffset.UtcNow;
}

public sealed class DnsRecordOwnershipEntity
{
    public Guid Id { get; set; } = Guid.NewGuid();

    public Guid ZoneId { get; set; }

    public DnsZoneEntity Zone { get; set; } = null!;

    public Guid? DnsRecordId { get; set; }

    public DnsRecordEntity? DnsRecord { get; set; }

    public Guid? ResourceId { get; set; }

    public ResourceEntity? Resource { get; set; }

    public string ProviderRecordId { get; set; } = string.Empty;

    public string Name { get; set; } = string.Empty;

    public string Type { get; set; } = string.Empty;

    public string Value { get; set; } = string.Empty;

    public string Ownership { get; set; } = DnsOwnershipNames.Unknown;

    public string OwnerWorkflow { get; set; } = "unknown";

    public string SyncState { get; set; } = DnsOwnershipSyncStateNames.Desired;

    public string? DesiredContentHash { get; set; }

    public string? AppliedContentHash { get; set; }

    public DateTimeOffset? LastObservedAtUtc { get; set; }

    public DateTimeOffset? LastAppliedAtUtc { get; set; }

    public DateTimeOffset CreatedAtUtc { get; set; } = DateTimeOffset.UtcNow;
}

public sealed class DnsImportDecisionEntity
{
    public Guid Id { get; set; } = Guid.NewGuid();

    public Guid ZoneId { get; set; }

    public DnsZoneEntity Zone { get; set; } = null!;

    public string ProviderRecordId { get; set; } = string.Empty;

    public string Name { get; set; } = string.Empty;

    public string Type { get; set; } = string.Empty;

    public string Value { get; set; } = string.Empty;

    public bool SelectedForImport { get; set; }

    public DateTimeOffset CreatedAtUtc { get; set; } = DateTimeOffset.UtcNow;
}

public static class ConnectionTypeNames
{
    public const string DnsProvider = ConnectionTypeContractNames.DnsProvider;
    public const string TraefikHost = ConnectionTypeContractNames.TraefikHost;
    public const string FirewallHost = ConnectionTypeContractNames.FirewallHost;
    public const string AdGuardHome = ConnectionTypeContractNames.AdGuardHome;
    public const string OidcProvider = ConnectionTypeContractNames.OidcProvider;
    public const string NotificationProvider = ConnectionTypeContractNames.NotificationProvider;
    public const string NetBirdManagement = ConnectionTypeContractNames.NetBirdManagement;
}

public static class ConnectionHealthStateNames
{
    public const string Unknown = "unknown";
    public const string Validating = "validating";
    public const string Healthy = "healthy";
    public const string Degraded = "degraded";
    public const string Failed = "failed";
}

public static class ConnectionDeletionPolicyNames
{
    public const string Required = "required";
    public const string Optional = "optional";
    public const string SystemLinked = "system_linked";
}

public static class DnsOwnershipNames
{
    public const string Unknown = "unknown";
    public const string Imported = "imported";
    public const string Managed = "managed";
    public const string System = "system";
    public const string User = "user";
}

public static class DnsOwnershipSyncStateNames
{
    public const string Desired = "desired";
    public const string Applied = "applied";
    public const string Drifted = "drifted";
    public const string Orphaned = "orphaned";
}

public static class DnsProviderTypeNames
{
    public const string Hetzner = "hetzner";
}
