namespace Hashi.Infrastructure.Persistence.Entities;

public sealed class ResourceEntity
{
    public Guid Id { get; set; } = Guid.NewGuid();

    public string Name { get; set; } = string.Empty;

    public string Slug { get; set; } = string.Empty;

    public string Kind { get; set; } = "https";

    public bool Enabled { get; set; } = true;

    public bool IsSystem { get; set; }

    public string Ownership { get; set; } = ResourceOwnershipNames.UserCreated;

    public string? OwningWorkflow { get; set; }

    public string DeletionPolicy { get; set; } = ResourceDeletionPolicyNames.Optional;

    public string SyncState { get; set; } = ResourceSyncStateNames.Desired;

    public string? LastAppliedHash { get; set; }

    public DateTimeOffset? LastAppliedAtUtc { get; set; }

    public string? Domain { get; set; }

    public string TargetScheme { get; set; } = "http";

    public string TargetHost { get; set; } = "127.0.0.1";

    public int TargetPort { get; set; } = 8080;

    public int? PublicPort { get; set; }

    public bool DashboardEnabled { get; set; }

    public bool StatusEnabled { get; set; }

    public string ForwardAuthPolicy { get; set; } = "adaptive";

    public string WafMode { get; set; } = "detect_only";

    public Guid? FirewallHostId { get; set; }

    public Guid? PulseAgentId { get; set; }

    public string? PathPrefix { get; set; }

    public string? PathRewrite { get; set; }

    public string ExtraMiddlewaresJson { get; set; } = "[]";

    public DateTimeOffset CreatedAtUtc { get; set; } = DateTimeOffset.UtcNow;

    public DateTimeOffset UpdatedAtUtc { get; set; } = DateTimeOffset.UtcNow;
}

public sealed class ResourceTargetEntity
{
    public Guid Id { get; set; } = Guid.NewGuid();

    public Guid ResourceId { get; set; }

    public ResourceEntity Resource { get; set; } = null!;

    public int Priority { get; set; }

    public string Scheme { get; set; } = "http";

    public string Host { get; set; } = "127.0.0.1";

    public int Port { get; set; } = 8080;

    public Guid? FirewallHostId { get; set; }

    public FirewallHostEntity? FirewallHost { get; set; }

    public Guid? PulseAgentId { get; set; }

    public bool Enabled { get; set; } = true;
}

public sealed class ResourcePortEntity
{
    public Guid Id { get; set; } = Guid.NewGuid();

    public Guid ResourceId { get; set; }

    public ResourceEntity Resource { get; set; } = null!;

    public int PublicPort { get; set; }

    public int TargetPort { get; set; }

    public string Protocol { get; set; } = "tcp";

    public string Ownership { get; set; } = ResourceOwnershipNames.UserCreated;

    public bool Confirmed { get; set; }

    public DateTimeOffset? ConfirmedAtUtc { get; set; }
}

public sealed class SystemResourceEntity
{
    public Guid Id { get; set; } = Guid.NewGuid();

    public Guid ResourceId { get; set; }

    public ResourceEntity Resource { get; set; } = null!;

    public string SystemKey { get; set; } = string.Empty;

    public string OwningWorkflow { get; set; } = "setup";

    public bool RequiredForAppAccess { get; set; } = true;

    public DateTimeOffset CreatedAtUtc { get; set; } = DateTimeOffset.UtcNow;
}

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

    public DateTimeOffset? LastCheckedAtUtc { get; set; }

    public int? LastLatencyMs { get; set; }
}

public sealed class PulseAgentEntity
{
    public Guid Id { get; set; } = Guid.NewGuid();

    public string Name { get; set; } = string.Empty;

    public string TokenHash { get; set; } = string.Empty;

    public DateTimeOffset? LastSeenAtUtc { get; set; }

    public string? LastPublicIp { get; set; }

    public string? LastPrivateIp { get; set; }

    public string? LastHostname { get; set; }

    public string? LastAgentVersion { get; set; }

    public DateTimeOffset? DnsPendingAtUtc { get; set; }

    public string Status { get; set; } = "pending";
}

public static class ResourceOwnershipNames
{
    public const string System = "system";
    public const string Managed = "managed";
    public const string Imported = "imported";
    public const string UserCreated = "user_created";
}

public static class ResourceDeletionPolicyNames
{
    public const string Optional = "optional";
    public const string OwningWorkflowOnly = "owning_workflow_only";
    public const string RequiredForAccess = "required_for_access";
}

public static class ResourceSyncStateNames
{
    public const string Desired = "desired";
    public const string Applied = "applied";
    public const string Drifted = "drifted";
}
