namespace Hashi.Contracts.Api;

public sealed record DnsWriteValidationRequest(bool ConfirmDryRun);

public sealed record DnsWriteValidationResponse(bool Valid, string? Error);

public sealed record DnsProviderValidationRequest(string ApiToken);

public sealed record DnsProviderValidationResponse(bool Valid, string? Error);

public sealed record CreateHetznerDnsConnectionRequest(
    string Name,
    string ApiToken,
    string ZoneName,
    int DefaultTtl);

public sealed record ConnectionSummaryResponse(
    Guid Id,
    string Name,
    string Type,
    bool Enabled,
    string HealthState,
    string? LastValidationMessage,
    DateTimeOffset? LastValidatedAtUtc);

public sealed record DnsZoneResponse(Guid Id, Guid ConnectionId, string ProviderZoneId, string Name, int DefaultTtl);

public sealed record DnsRecordResponse(
    Guid Id,
    string Name,
    string Type,
    string Value,
    int? Ttl,
    string Ownership,
    bool Enabled);

public sealed record DnsImportDecisionResponse(
    Guid Id,
    string ProviderRecordId,
    string Name,
    string Type,
    string Value,
    bool SelectedForImport);

public sealed record DnsImportApplyRequest(IReadOnlyList<Guid> SelectedDecisionIds);

public sealed record DnsPruneApplyRequest(bool ConfirmDestructive);

public sealed record DnsPlanChangeResponse(
    string Kind,
    string Name,
    string Type,
    string? CurrentValue,
    string? DesiredValue,
    int? Ttl,
    string RiskReason);

public sealed record DnsSyncPlanResponse(
    Guid PlanId,
    Guid ConnectionId,
    string ZoneName,
    IReadOnlyList<DnsPlanChangeResponse> Changes,
    bool RequiresConfirmation);

public sealed record DnsSyncApplyRequest(Guid PlanId, Guid ConnectionId, bool ConfirmDestructive);
