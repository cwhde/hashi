namespace Hashi.Infrastructure.Persistence.Entities;

public sealed class TraefikUserMiddlewareEntity
{
    public Guid Id { get; set; } = Guid.Parse("00000000-0000-0000-0000-000000000001");

    public string Yaml { get; set; } = """
        http:
          middlewares: {}
        """;

    public string? LastValidYaml { get; set; }

    public string? LastParseError { get; set; }

    public DateTimeOffset UpdatedAtUtc { get; set; } = DateTimeOffset.UtcNow;
}

public sealed class TraefikHostStateEntity
{
    public Guid Id { get; set; } = Guid.NewGuid();

    public Guid ConnectionId { get; set; }

    public string StaticConfigPath { get; set; } = "/etc/hashi/traefik/traefik.yml";

    public string DynamicConfigPath { get; set; } = "/etc/hashi/traefik/dynamic/http.yml";

    public string? LastAppliedContentHash { get; set; }

    public string? LastBackupStaticYaml { get; set; }

    public string? LastBackupDynamicYaml { get; set; }

    public DateTimeOffset? LastAppliedAtUtc { get; set; }
}

public sealed class FirewallHostEntity
{
    public Guid Id { get; set; } = Guid.NewGuid();

    public Guid ConnectionId { get; set; }

    public string Name { get; set; } = string.Empty;

    public string Domain { get; set; } = string.Empty;

    public string ManagedSubnetsJson { get; set; } = "[]";

    public string LinkedTraefikHost { get; set; } = string.Empty;

    public string InternalTraefikIp { get; set; } = string.Empty;

    public string? PublicIp { get; set; }

    public string? WanInterface { get; set; }

    public string? LxcBridge { get; set; }

    public string ScriptPath { get; set; } = "/opt/hashi/firewall/hashi-firewall.sh";

    public bool NetBirdEnabled { get; set; } = true;

    public string NetBirdInterface { get; set; } = "wt0";

    public string NetBirdOverlayCidrsJson { get; set; } = "[\"100.110.0.0/16\"]";

    public string NetBirdRoutedCidrsJson { get; set; } = "[]";

    public bool NetBirdRoutingPeer { get; set; }

    public int RollbackTimerSeconds { get; set; } = 300;

    public bool NetBirdDetected { get; set; }

    public string? LastAppliedScriptHash { get; set; }

    public string? RollbackScript { get; set; }

    public DateTimeOffset? LastAppliedAtUtc { get; set; }
}

public sealed class ResourceRouteEntity
{
    public Guid Id { get; set; } = Guid.NewGuid();

    public Guid ResourceId { get; set; }

    public bool Enabled { get; set; } = true;

    public int Priority { get; set; }

    public string PathMatchType { get; set; } = "prefix";

    public string PathValue { get; set; } = "/";

    public string TargetScheme { get; set; } = "http";

    public string TargetHost { get; set; } = "127.0.0.1";

    public int TargetPort { get; set; } = 8080;

    public string? RewriteMode { get; set; }

    public string? RewriteValue { get; set; }

    public string ExtraMiddlewaresJson { get; set; } = "[]";
}

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

public sealed class TraefikEntryPointEntity
{
    public Guid Id { get; set; } = Guid.NewGuid();

    public int Port { get; set; }

    public string Protocol { get; set; } = "tcp";

    public Guid? ResourceId { get; set; }

    public string? Label { get; set; }

    public bool Confirmed { get; set; }

    public DateTimeOffset? ConfirmedAtUtc { get; set; }

    public DateTimeOffset CreatedAtUtc { get; set; } = DateTimeOffset.UtcNow;
}

public sealed class MonitorEventEntity
{
    public Guid Id { get; set; } = Guid.NewGuid();

    public Guid MonitorEndpointId { get; set; }

    public string PreviousStatus { get; set; } = "unknown";

    public string NewStatus { get; set; } = "unknown";

    public int? LatencyMs { get; set; }

    public DateTimeOffset OccurredAtUtc { get; set; } = DateTimeOffset.UtcNow;
}

public sealed class SecurityEventEntity
{
    public Guid Id { get; set; } = Guid.NewGuid();

    public string Category { get; set; } = "access";

    public string Action { get; set; } = string.Empty;

    public string? ClientIp { get; set; }

    public string? Host { get; set; }

    public string? Path { get; set; }

    public string? DetailsJson { get; set; }

    public DateTimeOffset OccurredAtUtc { get; set; } = DateTimeOffset.UtcNow;
}

public sealed class EdgeSessionEntity
{
    public string SessionKey { get; set; } = string.Empty;

    public Guid OidcProviderId { get; set; }

    public string Subject { get; set; } = string.Empty;

    public DateTimeOffset ExpiresAtUtc { get; set; }

    public DateTimeOffset CreatedAtUtc { get; set; } = DateTimeOffset.UtcNow;
}

public sealed class MonitorSampleEntity
{
    public Guid Id { get; set; } = Guid.NewGuid();

    public Guid MonitorEndpointId { get; set; }

    public DateOnly PartitionDate { get; set; }

    public DateTimeOffset CheckedAtUtc { get; set; }

    public string Status { get; set; } = "unknown";

    public int LatencyMs { get; set; }
}

public sealed class MonitorRollupEntity
{
    public Guid Id { get; set; } = Guid.NewGuid();

    public Guid MonitorEndpointId { get; set; }

    public DateTimeOffset BucketStartUtc { get; set; }

    public int IntervalMinutes { get; set; } = 60;

    public int SampleCount { get; set; }

    public int UpCount { get; set; }

    public int DownCount { get; set; }

    public double AverageLatencyMs { get; set; }
}

public sealed class OidcProviderEntity
{
    public Guid Id { get; set; } = Guid.NewGuid();

    public string Name { get; set; } = string.Empty;

    public string Issuer { get; set; } = string.Empty;

    public string ClientId { get; set; } = string.Empty;

    public Guid ClientSecretId { get; set; }

    public string Scopes { get; set; } = "openid profile email";

    public bool Enabled { get; set; } = true;
}

public sealed class EdgeAuthRuleEntity
{
    public Guid Id { get; set; } = Guid.NewGuid();

    public string Name { get; set; } = string.Empty;

    public int Priority { get; set; }

    public string MatchJson { get; set; } = "{}";

    public string Action { get; set; } = "allow";

    public bool Enabled { get; set; } = true;
}

public sealed class AccessLogEventEntity
{
    public Guid Id { get; set; } = Guid.NewGuid();

    public DateTimeOffset ReceivedAtUtc { get; set; } = DateTimeOffset.UtcNow;

    public string ClientIp { get; set; } = string.Empty;

    public string Host { get; set; } = string.Empty;

    public string Path { get; set; } = string.Empty;

    public int StatusCode { get; set; }

    public string? CountryCode { get; set; }

    public string? Asn { get; set; }

    public string Decision { get; set; } = "allow";
}

public sealed class AbuseBucketEntity
{
    public Guid Id { get; set; } = Guid.NewGuid();

    public string ClientIp { get; set; } = string.Empty;

    public int Score { get; set; }

    public string State { get; set; } = "watch";

    public DateTimeOffset UpdatedAtUtc { get; set; } = DateTimeOffset.UtcNow;
}

public sealed class SecurityRequestBucketEntity
{
    public Guid Id { get; set; } = Guid.NewGuid();

    public DateTimeOffset BucketStartUtc { get; set; }

    public string ClientIp { get; set; } = string.Empty;

    public string Resource { get; set; } = string.Empty;

    public string TraefikInstance { get; set; } = "default";

    public string? CountryCode { get; set; }

    public string? RegionCode { get; set; }

    public string? Asn { get; set; }

    public int StatusClass { get; set; }

    public string Method { get; set; } = "GET";

    public string PathPrefix { get; set; } = "/";

    public long TotalCount { get; set; }

    public long AllowedCount { get; set; }

    public long BlockedCount { get; set; }

    public long ChallengedCount { get; set; }

    public DateTimeOffset UpdatedAtUtc { get; set; } = DateTimeOffset.UtcNow;
}

public sealed class AccessLogCursorEntity
{
    public Guid Id { get; set; } = Guid.NewGuid();

    public Guid ConnectionId { get; set; }

    public long ByteOffset { get; set; }

    public DateTimeOffset UpdatedAtUtc { get; set; } = DateTimeOffset.UtcNow;
}

public sealed class BlocklistEntryEntity
{
    public Guid Id { get; set; } = Guid.NewGuid();

    public string ClientIp { get; set; } = string.Empty;

    public string Reason { get; set; } = string.Empty;

    public bool SyncedToFirewall { get; set; }

    public DateTimeOffset CreatedAtUtc { get; set; } = DateTimeOffset.UtcNow;
}

public sealed class AdGuardConnectionEntity
{
    public Guid Id { get; set; } = Guid.NewGuid();

    public string Name { get; set; } = string.Empty;

    public string BaseUrl { get; set; } = string.Empty;

    public Guid PasswordSecretId { get; set; }

    public bool Enabled { get; set; } = true;
}

public sealed class AdGuardRewriteEntity
{
    public Guid Id { get; set; } = Guid.NewGuid();

    public Guid ConnectionId { get; set; }

    public string Domain { get; set; } = string.Empty;

    public string Answer { get; set; } = string.Empty;

    public bool ManagedByHashi { get; set; } = true;

    public string? ProviderRewriteId { get; set; }
}

public sealed class NotificationProviderEntity
{
    public Guid Id { get; set; } = Guid.NewGuid();

    public string Name { get; set; } = string.Empty;

    public string Type { get; set; } = "smtp";

    public string SettingsJson { get; set; } = "{}";

    public bool Enabled { get; set; } = true;
}

public sealed class ScriptEntity
{
    public Guid Id { get; set; } = Guid.NewGuid();

    public Guid ConnectionId { get; set; }

    public string Name { get; set; } = string.Empty;

    public string Description { get; set; } = string.Empty;

    public string Body { get; set; } = string.Empty;

    public string CronExpression { get; set; } = string.Empty;

    public bool Enabled { get; set; } = true;

    public DateTimeOffset? LastRunAtUtc { get; set; }

    public string? LastRunOutput { get; set; }
}
