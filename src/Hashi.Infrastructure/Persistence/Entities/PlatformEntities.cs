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

    public string DomainMode { get; set; } = "custom";

    public string? Domain { get; set; }

    public string TargetScheme { get; set; } = "http";

    public string TargetHost { get; set; } = "127.0.0.1";

    public int TargetPort { get; set; } = 8080;

    public int? PublicPort { get; set; }

    public bool? TcpProxyProtocolEnabled { get; set; }

    public string? MonitoringProtocolHint { get; set; }

    public bool DashboardEnabled { get; set; }

    public bool StatusEnabled { get; set; }

    public string ForwardAuthPolicy { get; set; } = "adaptive";

    public string WafMode { get; set; } = "detect_only";

    public string WafExclusionsJson { get; set; } = "[]";

    public Guid? FirewallHostId { get; set; }

    public Guid? PulseAgentId { get; set; }

    public string? PathPrefix { get; set; }

    public string? PathRewriteMode { get; set; }

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

public sealed class ConnectionTargetEntity
{
    public Guid Id { get; set; } = Guid.NewGuid();

    public string OwnerType { get; set; } = ConnectionTargetOwnerTypeNames.Connection;

    public Guid OwnerId { get; set; }

    public string TargetMode { get; set; } = ConnectionTargetModeNames.StaticHost;

    public string? StaticHost { get; set; }

    public string? StaticIp { get; set; }

    public Guid? PulseAgentId { get; set; }

    public PulseAgentEntity? PulseAgent { get; set; }

    public string PulseIpMode { get; set; } = PulseTargetIpModeNames.Selected;

    public string PrivateCandidateSelector { get; set; } = PulsePrivateCandidateSelectorNames.Selected;

    public int Port { get; set; } = 80;

    public string Scheme { get; set; } = "http";

    public string? PathPrefix { get; set; }

    public string TlsValidationMode { get; set; } = TlsValidationModeNames.System;

    public string? ExpectedTlsHostname { get; set; }

    public string? ResolvedIpSnapshot { get; set; }

    public DateTimeOffset? LastResolvedAtUtc { get; set; }

    public string Status { get; set; } = ConnectionTargetStatusNames.Unresolved;

    public string? LastError { get; set; }

    public DateTimeOffset CreatedAtUtc { get; set; } = DateTimeOffset.UtcNow;

    public DateTimeOffset UpdatedAtUtc { get; set; } = DateTimeOffset.UtcNow;
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

    public Guid? DnsRecordId { get; set; }

    public DateTimeOffset? LastCheckedAtUtc { get; set; }

    public int? LastLatencyMs { get; set; }
}

public sealed class PulseAgentEntity
{
    public Guid Id { get; set; } = Guid.NewGuid();

    public string Name { get; set; } = string.Empty;

    public string TokenHash { get; set; } = string.Empty;

    public string InstallType { get; set; } = "linux_service";

    public string AllowedScopesJson { get; set; } = """["heartbeat"]""";

    public int HeartbeatIntervalSeconds { get; set; } = 60;

    public DateTimeOffset? LastSeenAtUtc { get; set; }

    public string? LastPublicIp { get; set; }

    public string? LastPrivateIp { get; set; }

    public string LastPrivateIpv4CandidatesJson { get; set; } = "[]";

    public string LastPrivateIpv6CandidatesJson { get; set; } = "[]";

    public string? LastSelectedIp { get; set; }

    public string? LastSelectedInterface { get; set; }

    public string? LastHostname { get; set; }

    public string? LastAgentVersion { get; set; }

    public string? LastDockerMetadataJson { get; set; }

    public DateTimeOffset? DnsPendingAtUtc { get; set; }

    public string Status { get; set; } = "pending";
}

public sealed class PulseHeartbeatEntity
{
    public Guid Id { get; set; } = Guid.NewGuid();

    public Guid PulseAgentId { get; set; }

    public PulseAgentEntity? PulseAgent { get; set; }

    public DateTimeOffset ReceivedAtUtc { get; set; } = DateTimeOffset.UtcNow;

    public DateTimeOffset AgentTimestampUtc { get; set; }

    public string? RemotePublicIp { get; set; }

    public string Version { get; set; } = string.Empty;

    public string Hostname { get; set; } = string.Empty;

    public string PrivateIpv4CandidatesJson { get; set; } = "[]";

    public string PrivateIpv6CandidatesJson { get; set; } = "[]";

    public string? SelectedIp { get; set; }

    public string? SelectedInterface { get; set; }

    public string? DockerMetadataJson { get; set; }
}

public sealed class InternalAgentDnsSettingsEntity
{
    public int Id { get; set; } = 1;

    public bool Enabled { get; set; }

    public string Domain { get; set; } = "hashi.home.arpa";

    public bool KeepLastRewriteWhenAgentStale { get; set; } = true;

    public Guid? AdGuardConnectionId { get; set; }

    public string LastSyncStatus { get; set; } = "never_run";

    public string? LastAppliedHash { get; set; }

    public DateTimeOffset UpdatedAtUtc { get; set; } = DateTimeOffset.UtcNow;
}

public sealed class InternalAgentDnsAgentSettingsEntity
{
    public Guid Id { get; set; } = Guid.NewGuid();

    public Guid PulseAgentId { get; set; }

    public PulseAgentEntity PulseAgent { get; set; } = null!;

    public bool Enabled { get; set; } = true;

    public string? NameOverride { get; set; }

    public string IpMode { get; set; } = PulseTargetIpModeNames.Selected;

    public bool KeepLastRewriteWhenStale { get; set; } = true;

    public DateTimeOffset UpdatedAtUtc { get; set; } = DateTimeOffset.UtcNow;
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

public static class ConnectionTargetOwnerTypeNames
{
    public const string Connection = "connection";
    public const string AdGuardConnection = "adguard_connection";
    public const string Resource = "resource";
}

public static class ConnectionTargetModeNames
{
    public const string StaticHost = "static_host";
    public const string StaticIp = "static_ip";
    public const string PulseAgent = "pulse_agent";
}

public static class PulseTargetIpModeNames
{
    public const string Selected = "selected";
    public const string Public = "public";
    public const string Private = "private";
    public const string PrivateSelected = "private_selected";
    public const string PrivateCandidate = "private_candidate";
}

public static class PulsePrivateCandidateSelectorNames
{
    public const string Selected = "selected";
    public const string FirstIpv4 = "first_ipv4";
    public const string FirstIpv6 = "first_ipv6";
}

public static class TlsValidationModeNames
{
    public const string System = "system";
    public const string ExpectedHostname = "expected_hostname";
    public const string Skip = "skip";
}

public static class ConnectionTargetStatusNames
{
    public const string Unresolved = "unresolved";
    public const string Resolved = "resolved";
    public const string Stale = "stale";
    public const string Failed = "failed";
}
