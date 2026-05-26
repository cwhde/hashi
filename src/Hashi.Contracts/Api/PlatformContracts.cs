namespace Hashi.Contracts.Api;

public sealed record ResourceResponse(
    Guid Id,
    string Name,
    string Slug,
    string Kind,
    bool Enabled,
    bool IsSystem,
    string? Domain,
    string TargetScheme,
    string TargetHost,
    int TargetPort,
    int? PublicPort,
    bool DashboardEnabled,
    bool StatusEnabled,
    Guid? FirewallHostId,
    Guid? PulseAgentId,
    string? PathPrefix,
    string? PathRewrite,
    string ForwardAuthPolicy,
    string WafMode,
    IReadOnlyList<string> ExtraMiddlewares,
    IReadOnlyList<ResourceRouteResponse> Routes,
    IReadOnlyList<ResourceRuleResponse> Rules);

public sealed record ResourceRouteResponse(
    Guid Id,
    bool Enabled,
    int Priority,
    string PathMatchType,
    string PathValue,
    string TargetScheme,
    string TargetHost,
    int TargetPort,
    string? RewriteMode,
    string? RewriteValue,
    IReadOnlyList<string> ExtraMiddlewares);

public sealed record ResourceRuleResponse(
    Guid Id,
    bool Enabled,
    int Priority,
    string Action,
    string MatchType,
    string MatchValue);

public sealed record ResourceRouteRequest(
    bool Enabled,
    int Priority,
    string PathMatchType,
    string PathValue,
    string TargetScheme,
    string TargetHost,
    int TargetPort,
    string? RewriteMode = null,
    string? RewriteValue = null,
    IReadOnlyList<string>? ExtraMiddlewares = null);

public sealed record ResourceRuleRequest(
    bool Enabled,
    int Priority,
    string Action,
    string MatchType,
    string MatchValue);

public sealed record CreateResourceRequest(
    string Name,
    string Kind,
    string? Domain,
    string TargetScheme,
    string TargetHost,
    int TargetPort,
    bool DashboardEnabled,
    bool StatusEnabled,
    int? PublicPort = null,
    Guid? FirewallHostId = null,
    Guid? PulseAgentId = null,
    string? PathPrefix = null,
    string? PathRewrite = null,
    string? ForwardAuthPolicy = null,
    string? WafMode = null,
    IReadOnlyList<string>? ExtraMiddlewares = null,
    IReadOnlyList<ResourceRouteRequest>? Routes = null,
    IReadOnlyList<ResourceRuleRequest>? Rules = null);

public sealed record UpdateResourceRequest(
    string? Name,
    bool? Enabled,
    string? Domain,
    string? TargetScheme,
    string? TargetHost,
    int? TargetPort,
    bool? DashboardEnabled,
    bool? StatusEnabled,
    int? PublicPort = null,
    bool ClearPublicPort = false,
    Guid? FirewallHostId = null,
    bool ClearFirewallHostId = false,
    Guid? PulseAgentId = null,
    bool ClearPulseAgentId = false,
    string? PathPrefix = null,
    bool ClearPathPrefix = false,
    string? PathRewrite = null,
    bool ClearPathRewrite = false,
    string? ForwardAuthPolicy = null,
    string? WafMode = null,
    IReadOnlyList<string>? ExtraMiddlewares = null,
    bool ClearExtraMiddlewares = false,
    IReadOnlyList<ResourceRouteRequest>? Routes = null,
    IReadOnlyList<ResourceRuleRequest>? Rules = null);

public sealed record TraefikDynamicFilesResponse(
    string CoreYaml,
    string HttpResourcesYaml,
    string StreamResourcesYaml,
    string UserMiddlewaresYaml,
    string SecurityYaml,
    string HealthYaml);

public sealed record TraefikRenderResponse(
    string StaticConfigYaml,
    string DynamicHttpYaml,
    string ContentHash,
    TraefikDynamicFilesResponse? DynamicFiles = null);

public sealed record TraefikApplyRequest(
    Guid ConnectionId,
    string Host,
    int Port,
    string Username,
    string AuthMode,
    string? Password,
    string? PrivateKeyPem,
    string? PrivateKeyPassphrase);

public sealed record TraefikApplyResponse(bool Succeeded, string ContentHash, bool Skipped, string? Message);

public sealed record TraefikInstallRequest(
    Guid ConnectionId,
    string Host,
    int Port,
    string Username,
    string AuthMode,
    string? Password,
    string? PrivateKeyPem,
    string? PrivateKeyPassphrase);

public sealed record TraefikInstallResponse(bool Succeeded, string? Message);

public sealed record TraefikEntryPointResponse(
    Guid Id,
    int Port,
    string Protocol,
    Guid? ResourceId,
    string? Label,
    bool Confirmed,
    DateTimeOffset? ConfirmedAtUtc);

public sealed record CertificateSetupRequest(
    string AcmeEmail,
    string EabKeyId,
    string EabHmac,
    int DnsChallengeDelaySeconds,
    IReadOnlyList<string>? Resolvers);

public sealed record CertificateSetupResponse(
    string? AcmeEmail,
    bool HasEabCredentials,
    int DnsChallengeDelaySeconds,
    IReadOnlyList<string> Resolvers,
    bool HasDnsProvider);

public sealed record CertificateSetupValidateResponse(bool IsValid, IReadOnlyList<string> Errors);

public sealed record CertificateSetupSaveResponse(bool Saved, string? Error);

public sealed record TraefikUserMiddlewareResponse(
    string Yaml,
    string? LastParseError,
    IReadOnlyList<string> MiddlewareNames,
    DateTimeOffset UpdatedAtUtc);

public sealed record UpdateTraefikUserMiddlewareRequest(string Yaml);

public sealed record TraefikUserMiddlewareValidationRequest(string Yaml);

public sealed record TraefikUserMiddlewareValidationResponse(
    bool IsValid,
    string? Error,
    IReadOnlyList<string> MiddlewareNames);

public sealed record TraefikConfigValidationResponse(bool IsValid, IReadOnlyList<string> Errors);

public sealed record TraefikHostStateResponse(
    Guid ConnectionId,
    string? LastAppliedContentHash,
    string? CurrentContentHash,
    DateTimeOffset? LastAppliedAtUtc,
    bool HasBackup,
    bool HasPendingChanges,
    string? LastParseError);

public sealed record TraefikDetectExistingResponse(bool Found, string? Preview, string RemotePath);

public sealed record TraefikApplyConnectionRequest(bool ConfirmReplaceExisting);

public sealed record FirewallRenderRequest(
    string Name,
    string Domain,
    IReadOnlyList<string> ManagedSubnets,
    string LinkedTraefikHost,
    string InternalTraefikIp,
    string? PublicIp = null,
    string? WanInterface = null,
    string? LxcBridge = null,
    bool? NetBirdEnabled = null,
    string? NetBirdInterface = null,
    IReadOnlyList<string>? NetBirdOverlayCidrs = null,
    IReadOnlyList<string>? NetBirdRoutedCidrs = null,
    bool? NetBirdRoutingPeer = null,
    int? RollbackTimerSeconds = null);

public sealed record FirewallRenderResponse(string Script);

public sealed record FirewallApplyRequest(
    Guid FirewallHostId,
    string Host,
    int Port,
    string Username,
    string AuthMode,
    string? Password,
    string? PrivateKeyPem,
    string? PrivateKeyPassphrase);

public sealed record FirewallApplyResponse(bool Succeeded, bool Skipped, bool NetBirdDetected, string? Message);

public sealed record FirewallHostResponse(
    Guid Id,
    Guid ConnectionId,
    string Name,
    string Domain,
    string LinkedTraefikHost,
    string InternalTraefikIp,
    string? PublicIp,
    string? WanInterface,
    IReadOnlyList<string> ManagedSubnets,
    bool NetBirdEnabled,
    string NetBirdInterface,
    IReadOnlyList<string> NetBirdOverlayCidrs,
    IReadOnlyList<string> NetBirdRoutedCidrs,
    bool NetBirdRoutingPeer,
    int RollbackTimerSeconds,
    bool NetBirdDetected,
    DateTimeOffset? LastAppliedAtUtc);

public sealed record CreateFirewallHostRequest(
    Guid ConnectionId,
    string Name,
    string Domain,
    IReadOnlyList<string> ManagedSubnets,
    string LinkedTraefikHost,
    string InternalTraefikIp,
    string? PublicIp = null,
    string? WanInterface = null,
    string? LxcBridge = null,
    bool? NetBirdEnabled = null,
    string? NetBirdInterface = null,
    IReadOnlyList<string>? NetBirdOverlayCidrs = null,
    IReadOnlyList<string>? NetBirdRoutedCidrs = null,
    bool? NetBirdRoutingPeer = null,
    int? RollbackTimerSeconds = null);

public sealed record UpdateFirewallHostRequest(
    string? Name,
    string? Domain,
    IReadOnlyList<string>? ManagedSubnets,
    string? LinkedTraefikHost,
    string? InternalTraefikIp,
    string? PublicIp,
    bool ClearPublicIp = false,
    string? WanInterface = null,
    bool ClearWanInterface = false,
    string? LxcBridge = null,
    bool ClearLxcBridge = false,
    bool? NetBirdEnabled = null,
    string? NetBirdInterface = null,
    IReadOnlyList<string>? NetBirdOverlayCidrs = null,
    IReadOnlyList<string>? NetBirdRoutedCidrs = null,
    bool? NetBirdRoutingPeer = null,
    int? RollbackTimerSeconds = null);

public sealed record MonitorRollupResponse(
    Guid MonitorEndpointId,
    DateTimeOffset BucketStartUtc,
    int IntervalMinutes,
    int SampleCount,
    int UpCount,
    int DownCount,
    double AverageLatencyMs);

public sealed record PublicStatusStripBucket(DateTimeOffset BucketStartUtc, bool Up);

public sealed record PublicStatusItemResponse(
    string Name,
    string Status,
    int? LastLatencyMs,
    IReadOnlyList<PublicStatusStripBucket> RecentStrip);

public sealed record PublicStatusSummaryResponse(
    int TotalEndpoints,
    int UpCount,
    int DegradedCount,
    int DownCount,
    int FirewallHostCount,
    int FirewallHostsApplied);

public sealed record MonitorEventResponse(
    Guid Id,
    Guid MonitorEndpointId,
    string PreviousStatus,
    string NewStatus,
    int? LatencyMs,
    DateTimeOffset OccurredAtUtc);

public sealed record MonitoringSettingsResponse(
    int MonitorCheckIntervalSeconds,
    int MonitorCheckTimeoutSeconds,
    int MonitorSampleRetentionDays,
    int MonitorDegradedLatencyMs,
    DateTimeOffset? UpdatedAtUtc);

public sealed record MonitoringSettingsRequest(
    int? MonitorCheckIntervalSeconds,
    int? MonitorCheckTimeoutSeconds,
    int? MonitorSampleRetentionDays,
    int? MonitorDegradedLatencyMs);

public sealed record EdgeSsoSettingsResponse(
    int EdgeSsoSessionHours,
    DateTimeOffset? UpdatedAtUtc);

public sealed record EdgeSsoSettingsRequest(int? EdgeSsoSessionHours);

public sealed record OidcProviderResponse(
    Guid Id,
    string Name,
    string Issuer,
    string ClientId,
    string Scopes,
    bool Enabled);

public sealed record CreateOidcProviderRequest(
    string Name,
    string Issuer,
    string ClientId,
    string ClientSecret,
    string? Scopes,
    bool Enabled);

public sealed record UpdateOidcProviderRequest(
    string? Name,
    string? Issuer,
    string? ClientId,
    string? ClientSecret,
    string? Scopes,
    bool? Enabled);

public sealed record EdgeAuthRuleResponse(
    Guid Id,
    string Name,
    int Priority,
    string MatchJson,
    string Action,
    bool Enabled);

public sealed record CreateEdgeAuthRuleRequest(
    string Name,
    int Priority,
    string MatchJson,
    string Action,
    bool Enabled);

public sealed record UpdateEdgeAuthRuleRequest(
    string? Name,
    int? Priority,
    string? MatchJson,
    string? Action,
    bool? Enabled);

public sealed record AdGuardConnectionTestResponse(bool Connected, string? Error);

public sealed record RotatePulseAgentTokenResponse(Guid Id, string Name, string Token);

public sealed record MonitorEndpointResponse(
    Guid Id,
    string Name,
    string Url,
    string CheckType,
    bool Enabled,
    string Status,
    DateTimeOffset? LastCheckedAtUtc,
    int? LastLatencyMs);

public sealed record PulseInstallResponse(string LinuxInstallScript, string DockerRunCommand);

public sealed record PulseAgentResponse(
    Guid Id,
    string Name,
    string Status,
    DateTimeOffset? LastSeenAtUtc,
    string? LastPublicIp,
    string? LastHostname,
    string? LastAgentVersion,
    DateTimeOffset? DnsPendingAtUtc);

public sealed record PulseHeartbeatRequest(string Version, string Hostname, IReadOnlyList<string> PrivateIpv4Candidates);

public sealed record EdgeAuthForwardResponse(string Decision, string? RedirectUrl);

public sealed record SecurityRankItem
{
    public required string Label { get; init; }
    public required long Count { get; init; }
}

public sealed record SecurityDashboardResponse(
    long Allowed,
    long Blocked,
    long Challenged,
    int Hours,
    IReadOnlyList<string> TopBlockedIps,
    IReadOnlyList<SecurityRankItem> TopCountries,
    IReadOnlyList<SecurityRankItem> TopAsns,
    long BlocklistCount,
    long SecurityEventCount);

public sealed record BlocklistSyncResponse(
    bool Synced,
    int PendingEntries,
    int AppliedHosts,
    IReadOnlyList<string> Failures);

public sealed record ForwardAuthDecisionIngestRequest(
    string ClientIp,
    string Host,
    string Path,
    string Decision,
    string? CountryCode,
    string? Asn);

public sealed record ScriptResponse(
    Guid Id,
    string Name,
    bool Enabled,
    string Description,
    string CronExpression,
    DateTimeOffset? LastRunAtUtc,
    string? LastRunOutput);

public sealed record UpdateScriptRequest(
    string? Name,
    string? Description,
    string? Body,
    string? CronExpression,
    bool? Enabled);

public sealed record NotificationProviderResponse(Guid Id, string Name, string Type, bool Enabled);

public sealed record CreateNotificationProviderRequest(string Name, string Type, string SettingsJson, bool Enabled);

public sealed record UpdateNotificationProviderRequest(
    string? Name,
    string? Type,
    string? SettingsJson,
    bool? Enabled);

public sealed record NotificationTestRequest(string Subject, string Body);

public sealed record NotificationTestResponse(bool Sent, string? Error);

public sealed record SendNotificationRequest(string Subject, string Body, IReadOnlyList<string> ProviderTypes);

public sealed record AccessLogIngestRequest(
    string ClientIp,
    string Host,
    string Path,
    int StatusCode,
    string? CountryCode,
    string? Asn);

public sealed record AdGuardRewriteResponse(Guid Id, string Domain, string Answer, bool ManagedByHashi);

public sealed record UpsertAdGuardRewriteRequest(string Domain, string Answer);

public sealed record AdGuardConnectionResponse(Guid Id, string Name, string BaseUrl, bool Enabled);

public sealed record CreateAdGuardConnectionRequest(string Name, string BaseUrl, string Password);

public sealed record CreateScriptRequest(Guid ConnectionId, string Name, string Description, string Body, string CronExpression);

public sealed record RunScriptRequest(
    string? Host = null,
    int Port = 22,
    string? Username = null,
    string AuthMode = "password",
    string? Password = null,
    string? PrivateKeyPem = null,
    string? PrivateKeyPassphrase = null);

public sealed record RunScriptResponse(bool Succeeded, string Output, string? Error);

public sealed record CreatePulseAgentRequest(string Name);

public sealed record CreatePulseAgentResponse(Guid Id, string Name, string Token);

public sealed record PulseHeartbeatAuthRequest(string Token, string Version, string Hostname, IReadOnlyList<string> PrivateIpv4Candidates);
