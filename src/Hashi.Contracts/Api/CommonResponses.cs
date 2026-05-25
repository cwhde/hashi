namespace Hashi.Contracts.Api;

public sealed record HealthResponse(string Status, string Version, DateTimeOffset Timestamp);

public sealed record SetupStatusResponse(
    bool IsComplete,
    string CurrentStep,
    IReadOnlyList<string> CompletedSteps,
    DateTimeOffset? UpdatedAtUtc);

public sealed record AuditEventResponse(
    Guid Id,
    string Category,
    string Action,
    string? SubjectType,
    string? SubjectId,
    string Outcome,
    DateTimeOffset CreatedAtUtc);

public sealed record SyncRunResponse(
    Guid Id,
    string Subsystem,
    string Status,
    string? RiskLevel,
    DateTimeOffset StartedAtUtc,
    DateTimeOffset? CompletedAtUtc,
    string? ErrorSummary);
