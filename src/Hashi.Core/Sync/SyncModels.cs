namespace Hashi.Core.Sync;

public enum SyncRunStatus
{
    Pending,
    Planning,
    AwaitingConfirmation,
    Applying,
    Reconciling,
    Succeeded,
    Failed,
    Cancelled,
}

public enum SyncRiskLevel
{
    None,
    Low,
    Medium,
    High,
    Destructive,
}

public enum ProviderResultKind
{
    NoOp,
    Created,
    Updated,
    Deleted,
    Failed,
    Skipped,
}

public sealed record ProviderChange(
    string ResourceType,
    string ResourceKey,
    ProviderResultKind Kind,
    string? Summary);

public sealed record SyncPlanResult(
    SyncRiskLevel RiskLevel,
    IReadOnlyList<ProviderChange> Changes,
    string? PreviewMarkdown);
