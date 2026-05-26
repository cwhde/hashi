namespace Hashi.Infrastructure.Persistence.Entities;

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

    public string ScriptPath { get; set; } = "/opt/hashi/firewall/hashi-firewall.sh";

    public bool NetBirdDetected { get; set; }

    public string? LastAppliedScriptHash { get; set; }

    public string? RollbackScript { get; set; }

    public DateTimeOffset? LastAppliedAtUtc { get; set; }
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
