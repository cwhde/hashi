namespace Hashi.Contracts.Api;

public sealed record SyncRunResponse(
    Guid Id,
    string Subsystem,
    string Status,
    string? RiskLevel,
    DateTimeOffset StartedAtUtc,
    DateTimeOffset? CompletedAtUtc,
    string? ErrorSummary,
    IReadOnlyList<SyncStepResponse> Steps,
    IReadOnlyList<SyncDiffResponse> Diffs);

public sealed record SyncStepResponse(
    Guid Id,
    string Name,
    string Status,
    DateTimeOffset StartedAtUtc,
    DateTimeOffset? CompletedAtUtc,
    string? Message);

public sealed record SyncDiffResponse(
    Guid Id,
    string ResourceType,
    string ResourceKey,
    string ChangeKind,
    string? Summary);

public sealed record SyncPlanPreviewResponse(
    Guid PlanId,
    string Subsystem,
    string RiskLevel,
    bool RequiresConfirmation,
    IReadOnlyList<SyncDiffResponse> Changes,
    string? PreviewMarkdown,
    IReadOnlyList<string>? ValidationErrors = null);

public sealed record SyncApplyRequest(Guid PlanId, bool ConfirmDestructive);

public sealed record SyncApplyResponse(
    Guid RunId,
    bool Succeeded,
    string Status,
    string? Message);

public sealed record SyncReconcileResponse(
    Guid RunId,
    bool Succeeded,
    IReadOnlyList<string> SubsystemsReconciled);
