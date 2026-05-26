namespace Hashi.Infrastructure.Persistence.Entities;

public sealed class ResourceEntity
{
    public Guid Id { get; set; } = Guid.NewGuid();

    public string Name { get; set; } = string.Empty;

    public string Slug { get; set; } = string.Empty;

    public string Kind { get; set; } = "https";

    public bool Enabled { get; set; } = true;

    public bool IsSystem { get; set; }

    public string? Domain { get; set; }

    public string TargetScheme { get; set; } = "http";

    public string TargetHost { get; set; } = "127.0.0.1";

    public int TargetPort { get; set; } = 8080;

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

public sealed class MonitorEndpointEntity
{
    public Guid Id { get; set; } = Guid.NewGuid();

    public string Name { get; set; } = string.Empty;

    public string Url { get; set; } = string.Empty;

    public string CheckType { get; set; } = "https";

    public bool Enabled { get; set; } = true;

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

    public string Status { get; set; } = "pending";
}
