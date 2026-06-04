namespace Hashi.Contracts.Api;

public sealed record ResourceResponse(
    Guid Id,
    string Name,
    string Slug,
    string Kind,
    bool Enabled,
    bool IsSystem,
    string DomainMode,
    string? Domain,
    string? ResolvedDomain,
    string TargetScheme,
    string TargetHost,
    int TargetPort,
    int? PublicPort,
    bool? TcpProxyProtocolEnabled,
    string? MonitoringProtocolHint,
    bool DashboardEnabled,
    bool StatusEnabled,
    Guid? FirewallHostId,
    Guid? PulseAgentId,
    string? PathPrefix,
    string? PathRewriteMode,
    string? PathRewrite,
    string ForwardAuthPolicy,
    string WafMode,
    IReadOnlyList<string> ExtraMiddlewares,
    IReadOnlyList<ResourceRouteResponse> Routes,
    IReadOnlyList<ResourceRuleResponse> Rules,
    IReadOnlyList<string> WafExclusions);

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
    bool? TcpProxyProtocolEnabled = null,
    string? MonitoringProtocolHint = null,
    Guid? FirewallHostId = null,
    Guid? PulseAgentId = null,
    string? PathPrefix = null,
    string? PathRewrite = null,
    string? ForwardAuthPolicy = null,
    string? WafMode = null,
    IReadOnlyList<string>? ExtraMiddlewares = null,
    IReadOnlyList<ResourceRouteRequest>? Routes = null,
    IReadOnlyList<ResourceRuleRequest>? Rules = null,
    IReadOnlyList<string>? WafExclusions = null,
    string? DomainMode = null,
    string? PathRewriteMode = null);

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
    bool? TcpProxyProtocolEnabled = null,
    string? MonitoringProtocolHint = null,
    bool ClearMonitoringProtocolHint = false,
    string? DomainMode = null,
    bool ClearDomain = false,
    bool ClearPublicPort = false,
    Guid? FirewallHostId = null,
    bool ClearFirewallHostId = false,
    Guid? PulseAgentId = null,
    bool ClearPulseAgentId = false,
    string? PathPrefix = null,
    bool ClearPathPrefix = false,
    string? PathRewriteMode = null,
    bool ClearPathRewriteMode = false,
    string? PathRewrite = null,
    bool ClearPathRewrite = false,
    string? ForwardAuthPolicy = null,
    string? WafMode = null,
    IReadOnlyList<string>? ExtraMiddlewares = null,
    bool ClearExtraMiddlewares = false,
    IReadOnlyList<ResourceRouteRequest>? Routes = null,
    IReadOnlyList<ResourceRuleRequest>? Rules = null,
    IReadOnlyList<string>? WafExclusions = null,
    bool ClearWafExclusions = false);

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
    IReadOnlyList<string>? Resolvers,
    Guid? DnsProviderConnectionId);

public sealed record CertificateSetupResponse(
    string? AcmeEmail,
    bool HasEabCredentials,
    int DnsChallengeDelaySeconds,
    IReadOnlyList<string> Resolvers,
    bool HasDnsProvider,
    Guid? DnsProviderConnectionId);

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

public sealed record FirewallPlanChangeResponse(
    string Kind,
    string ResourceKey,
    string Summary);

public sealed record FirewallPlanPreviewResponse(
    Guid PlanId,
    Guid FirewallHostId,
    string ScriptHash,
    bool HasChanges,
    IReadOnlyList<FirewallPlanChangeResponse> Changes,
    string Preview);

public sealed record FirewallApplyResponse(
    bool Succeeded,
    bool Skipped,
    bool NetBirdDetected,
    string? Message,
    Guid? PlanId = null,
    string? ScriptHash = null,
    string? Preview = null);

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

public sealed record PublicDashboardResponse(
    IReadOnlyList<PublicDashboardItemResponse> Items,
    int HostsOnline,
    int TotalHosts,
    int LinuxFirewallHostsAvailable,
    int TotalLinuxFirewallHosts);

public sealed record PublicDashboardItemResponse(
    Guid Id,
    string Source,
    string DisplayName,
    string PublicUrl,
    string? Domain,
    string Status,
    int? LastLatencyMs);

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
    int EdgeSsoIdleTimeoutMinutes,
    int EdgeSsoRememberDeviceDays,
    DateTimeOffset? UpdatedAtUtc);

public sealed record EdgeSsoSettingsRequest(
    int? EdgeSsoSessionHours,
    int? EdgeSsoIdleTimeoutMinutes,
    int? EdgeSsoRememberDeviceDays);

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
    bool PublicStatusEnabled,
    string Status,
    DateTimeOffset? LastCheckedAtUtc,
    int? LastLatencyMs,
    Guid? ResourceId = null,
    string? ResourceType = null,
    string? Host = null,
    Guid? FirewallHostId = null,
    string? FirewallHostName = null,
    bool Provisioned = false);

public sealed record CreateMonitorEndpointRequest(
    string Name,
    string Url,
    string CheckType,
    bool Enabled = true,
    bool PublicStatusEnabled = false);

public sealed record UpdateMonitorEndpointRequest(
    string? Name = null,
    string? Url = null,
    string? CheckType = null,
    bool? Enabled = null,
    bool? PublicStatusEnabled = null);

public sealed record PulseInstallResponse(string LinuxInstallScript, string DockerComposeSnippet);

public sealed record PulseAgentResponse(
    Guid Id,
    string Name,
    string InstallType,
    IReadOnlyList<string> AllowedScopes,
    int HeartbeatIntervalSeconds,
    string Status,
    DateTimeOffset? LastSeenAtUtc,
    string? LastPublicIp,
    string? LastPrivateIp,
    IReadOnlyList<string> LastPrivateIpv4Candidates,
    IReadOnlyList<string> LastPrivateIpv6Candidates,
    string? LastSelectedIp,
    string? LastSelectedInterface,
    string? LastHostname,
    string? LastAgentVersion,
    DateTimeOffset? DnsPendingAtUtc);

public sealed record PulseDockerMetadataRequest(
    string? ContainerId,
    string? Image,
    string? NetworkMode);

public sealed record PulseHeartbeatRequest(
    string Version,
    string Hostname,
    IReadOnlyList<string> PrivateIpv4Candidates,
    IReadOnlyList<string> PrivateIpv6Candidates,
    string? SelectedInterface,
    string? SelectedIp,
    DateTimeOffset Timestamp,
    PulseDockerMetadataRequest? Docker);

public sealed record EdgeAuthForwardResponse(string Decision, string? RedirectUrl);

public sealed record SecurityRankItem
{
    public required string Label { get; init; }
    public required long Count { get; init; }
}

public sealed record SecurityResourceEnforcementItem
{
    public required string Resource { get; init; }
    public required long Blocked { get; init; }
    public required long Challenged { get; init; }
}

public sealed record SecurityRecentEventItem
{
    public required DateTimeOffset OccurredAtUtc { get; init; }
    public required string Category { get; init; }
    public required string Action { get; init; }
    public string? ClientIp { get; init; }
    public string? Host { get; init; }
    public string? Path { get; init; }
}

public sealed record SecurityFilterOption
{
    public required string Value { get; init; }
    public required string Label { get; init; }
}

public sealed record SecurityFirewallHostOption
{
    public required Guid Id { get; init; }
    public required string Name { get; init; }
    public required string Domain { get; init; }
    public required string LinkedTraefikHost { get; init; }
}

public sealed record SecurityTopBlockedIpItem
{
    public required string Ip { get; init; }
    public required long Count { get; init; }
    public required DateTimeOffset LastSeenAtUtc { get; init; }
    public string? CountryCode { get; init; }
    public string? Asn { get; init; }
    public string? Reason { get; init; }
    public DateTimeOffset? ExpiresAtUtc { get; init; }
    public string? SubjectState { get; init; }
}

public sealed record SecurityDashboardResponse(
    long Allowed,
    long Blocked,
    long Challenged,
    long WafDetections,
    long WafBlocks,
    int Hours,
    string? ResourceFilter,
    string? TraefikHostFilter,
    Guid? FirewallHostIdFilter,
    IReadOnlyList<SecurityTopBlockedIpItem> TopBlockedIps,
    IReadOnlyList<SecurityRankItem> TopCountries,
    IReadOnlyList<SecurityRankItem> TopAsns,
    IReadOnlyList<SecurityResourceEnforcementItem> TopResourcesBlockedChallenged,
    IReadOnlyList<SecurityRecentEventItem> RecentEvents,
    IReadOnlyList<SecurityFilterOption> ResourceFilters,
    IReadOnlyList<SecurityFilterOption> TraefikHostFilters,
    IReadOnlyList<SecurityFirewallHostOption> FirewallHostFilters,
    long FirewallActiveIpBlocks,
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
    string? Asn,
    string? RegionCode = null,
    string? Method = null,
    string? PathPrefix = null);

public sealed record WafEventIngestRequest(
    string ClientIp,
    string Host,
    string Path,
    string Action);

public sealed record ScriptResponse(
    Guid Id,
    Guid ConnectionId,
    string Name,
    bool Enabled,
    string Description,
    string CronExpression,
    int RunTimeoutSeconds,
    DateTimeOffset? LastRunAtUtc,
    string? LastRunOutput,
    string? LastRunError,
    string LastRunStatus,
    Guid? LastRunId,
    IReadOnlyList<ScriptTargetResponse> Targets,
    IReadOnlyList<ScriptEnvironmentVariableResponse> EnvironmentVariables);

public sealed record ScriptTargetResponse(Guid Id, Guid ConnectionId, bool Enabled);

public sealed record ScriptEnvironmentVariableResponse(Guid Id, string Name, bool IsSecret, Guid? SecretId);

public sealed record ScriptRunResponse(
    Guid Id,
    Guid ScriptId,
    Guid ConnectionId,
    DateTimeOffset StartedAtUtc,
    DateTimeOffset? CompletedAtUtc,
    string Status,
    bool Succeeded,
    string? Error);

public sealed record ScriptEnvironmentVariableRequest(
    string Name,
    string? Value = null,
    bool IsSecret = false,
    Guid? SecretId = null);

public sealed record UpdateScriptRequest(
    string? Name,
    string? Description,
    string? Body,
    string? CronExpression,
    bool? Enabled,
    IReadOnlyList<Guid>? TargetConnectionIds = null,
    int? RunTimeoutSeconds = null,
    IReadOnlyList<ScriptEnvironmentVariableRequest>? EnvironmentVariables = null);

public sealed record NotificationProviderResponse(Guid Id, string Name, string Type, bool Enabled);

public sealed record CreateNotificationProviderRequest(string Name, string Type, string SettingsJson, bool Enabled);

public sealed record UpdateNotificationProviderRequest(
    string? Name,
    string? Type,
    string? SettingsJson,
    bool? Enabled);

public sealed record NotificationTestRequest(string Subject, string Body);

public sealed record NotificationTestResponse(bool Sent, string? Error);

public sealed record TelegramChatDiscoveryRequest(string BotToken);

public sealed record TelegramChatDiscoveryResponse(
    bool Found,
    string? ChatId,
    string? ChatTitle,
    string? Error);

public sealed record SendNotificationRequest(string Subject, string Body, IReadOnlyList<string> ProviderTypes);

public sealed record AccessLogIngestRequest(
    string ClientIp,
    string Host,
    string Path,
    int StatusCode,
    string? CountryCode,
    string? Asn,
    string? RegionCode = null,
    string? Method = null,
    string? PathPrefix = null,
    string? TraefikInstance = null,
    string? Resource = null);

public sealed record AdGuardRewriteResponse(Guid Id, string Domain, string Answer, bool ManagedByHashi, string Source);

public sealed record AdGuardRewritePlanChangeResponse(
    string Kind,
    string Domain,
    string? CurrentAnswer,
    string? DesiredAnswer,
    string Summary);

public sealed record AdGuardRewritePlanResponse(
    Guid PlanId,
    Guid ConnectionId,
    bool RequiresConfirmation,
    IReadOnlyList<AdGuardRewritePlanChangeResponse> Changes);

public sealed record AdGuardRewriteMutationResponse(
    AdGuardRewriteResponse? Rewrite,
    AdGuardRewritePlanResponse Plan);

public sealed record AdGuardRewriteApplyRequest(Guid PlanId, bool ConfirmDestructive = false);

public sealed record AdGuardRewriteApplyResponse(Guid RunId, bool Succeeded, string Status, string? Message);

public sealed record UpsertAdGuardRewriteRequest(string Domain, string Answer);

public sealed record AdGuardConnectionResponse(Guid Id, string Name, string BaseUrl, bool Enabled);

public sealed record CreateAdGuardConnectionRequest(string Name, string BaseUrl, string Password);

public sealed record CreateScriptRequest(
    Guid ConnectionId,
    string Name,
    string Description,
    string Body,
    string CronExpression,
    IReadOnlyList<Guid>? TargetConnectionIds = null,
    int RunTimeoutSeconds = 300,
    IReadOnlyList<ScriptEnvironmentVariableRequest>? EnvironmentVariables = null);

public sealed record RunScriptRequest(
    string? Host = null,
    int Port = 22,
    string? Username = null,
    string AuthMode = "password",
    string? Password = null,
    string? PrivateKeyPem = null,
    string? PrivateKeyPassphrase = null);

public sealed record RunScriptResponse(
    bool Succeeded,
    string Output,
    string? Error,
    string Status = "unknown",
    Guid? RunId = null,
    IReadOnlyList<ScriptRunResponse>? Runs = null);

public sealed record CreatePulseAgentRequest(
    string Name,
    string InstallType = "linux_service",
    IReadOnlyList<string>? AllowedScopes = null,
    int? HeartbeatIntervalSeconds = null);

public sealed record CreatePulseAgentResponse(Guid Id, string Name, string Token);

public sealed record PulseHeartbeatAuthRequest(
    string Token,
    string Version,
    string Hostname,
    IReadOnlyList<string> PrivateIpv4Candidates,
    IReadOnlyList<string> PrivateIpv6Candidates,
    string? SelectedInterface,
    string? SelectedIp,
    DateTimeOffset Timestamp,
    PulseDockerMetadataRequest? Docker);
