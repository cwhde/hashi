namespace Hashi.Contracts.Api;

public sealed record HealthResponse(string Status, string Version, DateTimeOffset Timestamp);

public sealed record SetupStatusResponse(
    bool IsComplete,
    string CurrentStep,
    IReadOnlyList<string> CompletedSteps,
    bool HttpsDomainVerified,
    DateTimeOffset? UpdatedAtUtc);

public sealed record SetupVerifyHttpsResponse(bool Verified, string? Error);

public sealed record AuditEventResponse(
    Guid Id,
    string Category,
    string Action,
    string? SubjectType,
    string? SubjectId,
    string Outcome,
    DateTimeOffset CreatedAtUtc);

public sealed record BootstrapAllowedResponse(bool Allowed, string? RemoteIp);

public sealed record GeneralSettingsResponse(
    string? RootDomain,
    string? AdminDomain,
    string? InternalUrl,
    int DefaultSyncIntervalMinutes,
    bool PublicDashboardEnabled,
    bool PublicStatusEnabled,
    string? Theme,
    DateTimeOffset? UpdatedAtUtc);

public sealed record GeneralSettingsRequest(
    string? RootDomain,
    string? AdminDomain,
    string? InternalUrl,
    int? DefaultSyncIntervalMinutes,
    bool? PublicDashboardEnabled,
    bool? PublicStatusEnabled,
    string? Theme);

public sealed record GeneralSettingsUpdateResponse(bool Updated, DateTimeOffset UpdatedAtUtc);

public sealed record ApiErrorResponse(string Error);

public sealed record SystemResourceSyncResponse(
    bool Succeeded,
    Guid RunId,
    string? RiskLevel,
    bool RequiresConfirmation,
    string? PreviewMarkdown,
    string? Message);
