namespace Hashi.Infrastructure.Persistence.Entities;

public sealed class AppSettingsEntity
{
    public int Id { get; set; } = 1;

    public string? RootDomain { get; set; }

    public string? AdminDomain { get; set; }

    public string? InternalUrl { get; set; }

    public int DefaultSyncIntervalMinutes { get; set; } = 60;

    public bool PublicDashboardEnabled { get; set; } = true;

    public bool PublicStatusEnabled { get; set; } = true;

    public string Theme { get; set; } = "dark";

    public DateTimeOffset UpdatedAtUtc { get; set; } = DateTimeOffset.UtcNow;
}

public sealed class SetupStateEntity
{
    public int Id { get; set; } = 1;

    public bool IsComplete { get; set; }

    public string CurrentStep { get; set; } = "bootstrap-access";

    public string CompletedStepsJson { get; set; } = "[]";

    public string? BootstrapUsername { get; set; }

    public string? BootstrapPasswordHash { get; set; }

    public DateTimeOffset? HttpsDomainVerifiedAtUtc { get; set; }

    public DateTimeOffset UpdatedAtUtc { get; set; } = DateTimeOffset.UtcNow;
}

public sealed class AuditEventEntity
{
    public Guid Id { get; set; } = Guid.NewGuid();

    public string Category { get; set; } = string.Empty;

    public string Action { get; set; } = string.Empty;

    public string? SubjectType { get; set; }

    public string? SubjectId { get; set; }

    public string Outcome { get; set; } = "success";

    public string? MetadataJson { get; set; }

    public DateTimeOffset CreatedAtUtc { get; set; } = DateTimeOffset.UtcNow;
}

public sealed class SyncRunEntity
{
    public Guid Id { get; set; } = Guid.NewGuid();

    public string Subsystem { get; set; } = string.Empty;

    public string Status { get; set; } = SyncRunStatusNames.Pending;

    public string? RiskLevel { get; set; }

    public DateTimeOffset StartedAtUtc { get; set; } = DateTimeOffset.UtcNow;

    public DateTimeOffset? CompletedAtUtc { get; set; }

    public string? ErrorSummary { get; set; }

    public ICollection<SyncStepEntity> Steps { get; set; } = new List<SyncStepEntity>();

    public ICollection<SyncDiffEntity> Diffs { get; set; } = new List<SyncDiffEntity>();
}

public sealed class SyncStepEntity
{
    public Guid Id { get; set; } = Guid.NewGuid();

    public Guid SyncRunId { get; set; }

    public SyncRunEntity SyncRun { get; set; } = null!;

    public string Name { get; set; } = string.Empty;

    public string Status { get; set; } = SyncRunStatusNames.Pending;

    public DateTimeOffset StartedAtUtc { get; set; } = DateTimeOffset.UtcNow;

    public DateTimeOffset? CompletedAtUtc { get; set; }

    public string? Message { get; set; }
}

public sealed class SyncDiffEntity
{
    public Guid Id { get; set; } = Guid.NewGuid();

    public Guid SyncRunId { get; set; }

    public SyncRunEntity SyncRun { get; set; } = null!;

    public string ResourceType { get; set; } = string.Empty;

    public string ResourceKey { get; set; } = string.Empty;

    public string ChangeKind { get; set; } = string.Empty;

    public string? Summary { get; set; }

    public string? BeforeJson { get; set; }

    public string? AfterJson { get; set; }
}

public sealed class BackgroundJobEntity
{
    public string JobKey { get; set; } = string.Empty;

    public string DisplayName { get; set; } = string.Empty;

    public string Status { get; set; } = "idle";

    public DateTimeOffset? LastStartedAtUtc { get; set; }

    public DateTimeOffset? LastCompletedAtUtc { get; set; }

    public DateTimeOffset? NextRunAtUtc { get; set; }

    public long? LastDurationMs { get; set; }

    public string? LastDiffSummary { get; set; }

    public string? LastError { get; set; }

    public int IntervalSeconds { get; set; } = 60;

    public DateTimeOffset UpdatedAtUtc { get; set; } = DateTimeOffset.UtcNow;
}

public static class SyncRunStatusNames
{
    public const string Pending = "pending";
    public const string Planning = "planning";
    public const string AwaitingConfirmation = "awaiting_confirmation";
    public const string Applying = "applying";
    public const string Reconciling = "reconciling";
    public const string Succeeded = "succeeded";
    public const string Failed = "failed";
    public const string Cancelled = "cancelled";
}
