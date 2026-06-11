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
    IReadOnlyList<string> WafExclusions,
    Guid? OidcProviderId = null,
    bool ErrorHandlingEnabled = true,
    bool AdGuardRewriteEnabled = true,
    string? ExplicitRoutingOverride = null,
    string? SecurityProfileName = null);

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
    string? PathRewriteMode = null,
    Guid? OidcProviderId = null,
    bool? ErrorHandlingEnabled = null,
    bool AdGuardRewriteEnabled = true,
    string? ExplicitRoutingOverride = null,
    string? SecurityProfileName = null);

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
    bool ClearWafExclusions = false,
    Guid? OidcProviderId = null,
    bool ClearOidcProviderId = false,
    bool? ErrorHandlingEnabled = null,
    bool? AdGuardRewriteEnabled = null,
    string? ExplicitRoutingOverride = null,
    bool ClearExplicitRoutingOverride = false,
    string? SecurityProfileName = null,
    bool ClearSecurityProfileName = false);

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
    DateTimeOffset? ConfirmedAtUtc,
    bool PendingRemoval);

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
    string? PrivateKeyPassphrase,
    bool AcknowledgeSshBlockRisk = false);

public sealed record FirewallPlanChangeResponse(
    string Kind,
    string ResourceKey,
    string Summary);

public sealed record DnsProviderCapabilitiesResponse(
    IReadOnlyList<string> SupportedRecordTypes,
    bool SupportsBatchOperations,
    int? MaxRecordsPerZone,
    bool SupportsComments,
    int? RateLimitLimit,
    int? RateLimitWindowSeconds);

public sealed record FirewallPlanPreviewResponse(
    Guid PlanId,
    Guid FirewallHostId,
    string ScriptHash,
    bool HasChanges,
    IReadOnlyList<FirewallPlanChangeResponse> Changes,
    string Preview,
    bool SshBlockRisk = false,
    string? SshBlockWarningMessage = null);

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
    DateTimeOffset? LastCheckedAtUtc,
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
    bool Enabled,
    bool IsDefault);

public sealed record CreateOidcProviderRequest(
    string Name,
    string Issuer,
    string ClientId,
    string ClientSecret,
    string? Scopes,
    bool Enabled,
    bool IsDefault = false);

public sealed record UpdateOidcProviderRequest(
    string? Name,
    string? Issuer,
    string? ClientId,
    string? ClientSecret,
    string? Scopes,
    bool? Enabled,
    bool? IsDefault = null);

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

public sealed record AdGuardConnectionTestResponse(
    bool Connected,
    string? Error,
    ConnectionTargetResponse? Target = null,
    string? ResolvedBaseUrl = null,
    bool TargetStale = false);

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
    bool? PublicStatusEnabled = null,
    bool? Paused = null);

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

public sealed record SecurityBlocklistMatchBucket
{
    public required DateTimeOffset BucketStartUtc { get; init; }
    public required long Count { get; init; }
}

public sealed record SecurityCaptchaOutcomeSummary
{
    public required long Solved { get; init; }
    public required long Failed { get; init; }
    public required long Ignored { get; init; }
}

public sealed record SecurityActiveBlockItem
{
    public required string SubjectType { get; init; }
    public required string SubjectValue { get; init; }
    public required string BlockType { get; init; }
    public string? Reason { get; init; }
    public DateTimeOffset? ExpiresAtUtc { get; init; }
    public DateTimeOffset LastSeenAtUtc { get; init; }
    public bool FirewallSynced { get; init; }
}

public sealed record SecurityStaleBlocklistSourceItem
{
    public required Guid Id { get; init; }
    public required string Name { get; init; }
    public required string LastFetchStatus { get; init; }
    public string? LastFetchError { get; init; }
    public DateTimeOffset? LastFetchedAtUtc { get; init; }
    public DateTimeOffset StaleSinceUtc { get; init; }
}

public sealed record SecurityGeoIpStatusSummary
{
    public required bool Enabled { get; init; }
    public required bool DatabaseAvailable { get; init; }
    public required bool IsStale { get; init; }
    public required string LastUpdateStatus { get; init; }
    public string? LastUpdateMessage { get; init; }
    public DateTimeOffset? LastUpdateAtUtc { get; init; }
    public DateTimeOffset? NextUpdateAtUtc { get; init; }
    public IReadOnlyList<string> MissingDatabases { get; init; } = [];
    public IReadOnlyList<string> StaleDatabases { get; init; } = [];
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
    IReadOnlyList<SecurityTopBlockedIpItem> TopChallengedIps,
    IReadOnlyList<SecurityRankItem> TopCountries,
    IReadOnlyList<SecurityRankItem> TopAsns,
    IReadOnlyList<SecurityResourceEnforcementItem> TopResourcesBlockedChallenged,
    IReadOnlyList<SecurityRecentEventItem> RecentEvents,
    IReadOnlyList<SecurityRecentEventItem> RecentManualActions,
    IReadOnlyList<SecurityBlocklistMatchBucket> BlocklistMatchesOverTime,
    SecurityCaptchaOutcomeSummary CaptchaOutcomes,
    IReadOnlyList<SecurityActiveBlockItem> ActiveSoftBlocks,
    IReadOnlyList<SecurityActiveBlockItem> ActiveFirewallBlocks,
    IReadOnlyList<SecurityStaleBlocklistSourceItem> StaleBlocklistSources,
    SecurityGeoIpStatusSummary GeoIpStatus,
    IReadOnlyList<SecurityFilterOption> ResourceFilters,
    IReadOnlyList<SecurityFilterOption> TraefikHostFilters,
    IReadOnlyList<SecurityFirewallHostOption> FirewallHostFilters,
    long FirewallActiveIpBlocks,
    long BlocklistCount,
    long SecurityEventCount);

public sealed record SecuritySubjectSummaryResponse(
    Guid Id,
    string SubjectType,
    string SubjectValue,
    string NormalizedValue,
    string CurrentState,
    DateTimeOffset FirstSeenAtUtc,
    DateTimeOffset LastSeenAtUtc,
    string? LastCountry,
    string? LastRegion,
    string? LastAsn,
    string? LastAsOrg);

public sealed record SecuritySubjectStateResponse(
    Guid SecuritySubjectId,
    bool ChallengeRequired,
    DateTimeOffset? ChallengeRequiredSinceUtc,
    string? ChallengeReason,
    Guid? ChallengeResourceId,
    int ChallengeAttempts,
    int RequestsWhileChallenged,
    int FailedChallengeCount,
    int SuccessfulChallengeCount,
    DateTimeOffset? LastChallengeSolvedAtUtc,
    DateTimeOffset? SoftBlockedUntilUtc,
    DateTimeOffset? FirewallBlockedUntilUtc,
    bool ManualAllowActive,
    bool ManualBlockActive,
    string? LastEscalationReason,
    DateTimeOffset? LastEscalationAtUtc,
    DateTimeOffset UpdatedAtUtc);

public sealed record SecuritySubjectDetailResponse(
    SecuritySubjectSummaryResponse Subject,
    SecuritySubjectStateResponse? State,
    IReadOnlyList<ManualSecurityEntryResponse> ManualEntries,
    IReadOnlyList<BlocklistEntryResponse> BlocklistEntries,
    IReadOnlyList<ResourceRuleResponse> ResourceRules,
    IReadOnlyList<SecurityFirewallApplicationResponse> FirewallApplications);

public sealed record SecuritySubjectSearchResponse(
    string Query,
    IReadOnlyList<SecuritySubjectSummaryResponse> Results);

public sealed record SecurityEffectiveDecisionResponse(
    Guid SubjectId,
    string Decision,
    string Action,
    string Reason,
    IReadOnlyList<string> Explanation,
    IReadOnlyList<Guid> MatchedManualEntryIds,
    IReadOnlyList<Guid> MatchedBlocklistEntryIds,
    IReadOnlyList<Guid> MatchedResourceRuleIds,
    SecuritySubjectStateResponse? State);

public sealed record SecurityFirewallApplicationResponse(
    Guid? FirewallHostId,
    string? FirewallHostName,
    string Enforcement,
    string Status,
    DateTimeOffset? AppliedAtUtc,
    string? LastError);

public sealed record SecurityEventResponse(
    Guid Id,
    DateTimeOffset OccurredAtUtc,
    string? SubjectType,
    string? SubjectValue,
    string? NormalizedSubjectValue,
    Guid? ResourceId,
    Guid? ConnectionId,
    string? EventType,
    string? Severity,
    string? Decision,
    string? Source,
    string? Reason,
    string? RequestMethod,
    string? RequestPath,
    int? StatusCode,
    string? RequestId,
    string? UserAgentHash,
    string? MetadataJson);

public sealed record SecurityRequestBucketResponse(
    Guid Id,
    DateTimeOffset BucketStartUtc,
    int BucketSizeSeconds,
    string SubjectType,
    string NormalizedSubjectValue,
    Guid? ResourceId,
    string? RootDomain,
    string? Country,
    string? Region,
    string? Asn,
    string Method,
    string PathPrefix,
    int StatusClass,
    long RequestCount,
    long BlockedCount,
    long ChallengedCount,
    long ChallengeIgnoredCount,
    long FailedChallengeCount);

public sealed record ManualSecurityEntryResponse(
    Guid Id,
    string SubjectType,
    string SubjectValue,
    string NormalizedValue,
    string EntryType,
    string ScopeType,
    string? ScopeId,
    string? Reason,
    Guid? CreatedByAdminId,
    DateTimeOffset CreatedAtUtc,
    DateTimeOffset? ExpiresAtUtc,
    bool IsPermanent,
    bool BypassBlocking,
    bool BypassAdaptiveEscalation,
    bool BypassRateLimit,
    bool BypassChallenge,
    bool BypassSso,
    bool Enabled,
    DateTimeOffset? LastHitAtUtc);

public sealed record UpsertManualSecurityEntryRequest(
    string SubjectType,
    string SubjectValue,
    string EntryType,
    string ScopeType,
    string? ScopeId,
    string? Reason,
    DateTimeOffset? ExpiresAtUtc,
    bool? IsPermanent,
    bool? BypassBlocking,
    bool? BypassAdaptiveEscalation,
    bool? BypassRateLimit,
    bool? BypassChallenge,
    bool? BypassSso,
    bool? Enabled);

public sealed record UpdateManualSecurityEntryRequest(
    string? Reason,
    DateTimeOffset? ExpiresAtUtc,
    bool? IsPermanent,
    bool? BypassBlocking,
    bool? BypassAdaptiveEscalation,
    bool? BypassRateLimit,
    bool? BypassChallenge,
    bool? BypassSso,
    bool? Enabled);

public sealed record CreateSecurityBlockRequest(
    string SubjectType,
    string SubjectValue,
    string BlockType,
    string? Reason,
    DateTimeOffset? ExpiresAtUtc,
    bool? IsPermanent,
    bool FirewallEnforced = false);

public sealed record UpdateSecurityBlockRequest(
    string? Reason,
    DateTimeOffset? ExpiresAtUtc,
    bool? IsPermanent,
    bool? Enabled,
    bool? FirewallEnforced);

public sealed record SecurityBlockDurationRequest(int DurationSeconds);

public sealed record SecurityBlockMutationResponse(
    ManualSecurityEntryResponse ManualEntry,
    SecuritySubjectStateResponse? State,
    bool FirewallSyncRecommended,
    FirewallPlanPreviewResponse? FirewallPreview);

public sealed record BanDurationPolicyContract(
    string PolicyType,
    int BaseDurationSeconds,
    decimal LinearMultiplier,
    decimal ExponentialMultiplier,
    int? MaxDurationSeconds,
    int? PermanentAfterCount,
    int CountWindowSeconds,
    int ResetCountAfterSeconds);

public sealed record SecurityPolicySettingsResponse(
    BanDurationPolicyContract DefaultSoftBlockPolicy,
    BanDurationPolicyContract DefaultFirewallBlockPolicy,
    BanDurationPolicyContract RepeatOffenderPolicy,
    int ChallengeIgnoredThreshold,
    int ChallengeIgnoredWindowSeconds,
    int FirewallBlockThresholdWhileChallenged,
    bool CaptchaSuccessDecaysTriggeringBuckets,
    int CaptchaSuccessBucketDecayPercent,
    DateTimeOffset UpdatedAtUtc);

public sealed record BlocklistSourceResponse(
    Guid Id,
    string Name,
    string SourceUrl,
    string Description,
    string Format,
    string EnforcementMode,
    bool CanFirewallEnforce,
    bool Enabled,
    bool AllowHttp,
    int RefreshIntervalHours,
    string LastFetchStatus,
    string? LastFetchError,
    DateTimeOffset? LastFetchedAtUtc,
    DateTimeOffset? LastSuccessAtUtc,
    int? LastHttpStatusCode,
    long EntryCount,
    int RejectedCount,
    bool IsStale,
    string? MetadataJson,
    DateTimeOffset CreatedAtUtc,
    DateTimeOffset UpdatedAtUtc);

public sealed record UpsertBlocklistSourceRequest(
    string Name,
    string SourceUrl,
    string? Description,
    string Format,
    string EnforcementMode,
    bool CanFirewallEnforce,
    bool Enabled,
    bool AllowHttp,
    int? RefreshIntervalHours,
    int? CsvColumnIndex = null,
    string? JsonArrayField = null,
    string? JsonValueField = null);

public sealed record BlocklistFetchPreviewResponse(
    Guid SourceId,
    string SourceName,
    int ParsedCount,
    int IgnoredCount,
    int ErrorCount,
    string? ContentHash,
    bool NotModified,
    IReadOnlyList<BlocklistPreviewEntryResponse> Entries,
    IReadOnlyList<string> Errors,
    IReadOnlyList<string> Warnings);

public sealed record BlocklistPreviewEntryResponse(
    string SubjectType,
    string Value,
    string NormalizedValue,
    int? LineNumber);

public sealed record BlocklistSourceMutationResponse(
    BlocklistSourceResponse Source,
    BlocklistFetchRunResponse? Run,
    BlocklistFetchPreviewResponse? Preview,
    bool FirewallSyncRecommended,
    int PendingFirewallEntryCount,
    IReadOnlyList<string> Warnings);

public sealed record BlocklistEntryResponse(
    Guid Id,
    Guid? SourceId,
    string SubjectType,
    string Value,
    string NormalizedValue,
    string Scope,
    string Type,
    string Reason,
    string Source,
    bool Enabled,
    string EnforcementMode,
    bool SyncedToFirewall,
    DateTimeOffset CreatedAtUtc,
    DateTimeOffset FirstSeenAtUtc,
    DateTimeOffset LastSeenAtUtc,
    DateTimeOffset? ExpiresAtUtc,
    DateTimeOffset? LastHitAtUtc,
    string? MetadataJson);

public sealed record BlocklistFetchRunResponse(
    Guid Id,
    Guid BlocklistSourceId,
    DateTimeOffset StartedAtUtc,
    DateTimeOffset? CompletedAtUtc,
    string Status,
    int? HttpStatusCode,
    int EntryCount,
    int AddedCount,
    int RemovedCount,
    int UnchangedCount,
    int RejectedCount,
    string? ContentHash,
    string? Error);

public sealed record CaptchaSettingsResponse(
    bool Enabled,
    string? PublicChallengeBaseUrl,
    string? SiteKey,
    bool HasSecretKey,
    int VerificationTimeoutSeconds,
    bool InstrumentationExpected,
    bool HeadlessDetectionExpected,
    Guid? CapAdminResourceId,
    string? CapAdminDomain,
    Guid? PublicChallengeResourceId,
    string? PublicChallengeDomain,
    string ChallengeResetMode,
    int ChallengeDecayPercent,
    int MinimumRepeatChallengeSeconds,
    int MaximumFailuresBeforeEscalation,
    int MaximumRequestsWhileChallenged,
    DateTimeOffset UpdatedAtUtc);

public sealed record CaptchaSettingsRequest(
    bool Enabled,
    string? PublicChallengeBaseUrl,
    string? SiteKey,
    string? SecretKey,
    Guid? SecretKeySecretId,
    int? VerificationTimeoutSeconds,
    bool? InstrumentationExpected,
    bool? HeadlessDetectionExpected,
    Guid? CapAdminResourceId,
    string? CapAdminDomain,
    Guid? PublicChallengeResourceId,
    string? PublicChallengeDomain,
    string? ChallengeResetMode,
    int? ChallengeDecayPercent,
    int? MinimumRepeatChallengeSeconds,
    int? MaximumFailuresBeforeEscalation,
    int? MaximumRequestsWhileChallenged);

public sealed record CaptchaTestRequest(string Token);

public sealed record CaptchaTestResponse(
    bool Succeeded,
    string Status,
    string? Error);

public sealed record CaptchaChallengeStatusResponse(
    bool Enabled,
    bool ChallengeRequired,
    string? Reason,
    string? SiteKey,
    string? CapApiEndpoint,
    string? ReturnUrl,
    string SafeReturnUrl);

public sealed record CaptchaChallengeVerifyRequest(
    string Token,
    string? ReturnUrl);

public sealed record CaptchaChallengeVerifyResponse(
    bool Verified,
    bool ChallengeCleared,
    string Status,
    string? RedirectUrl,
    string? Error);

public sealed record ConnectionTargetResponse(
    Guid Id,
    string OwnerType,
    Guid OwnerId,
    string TargetMode,
    string? StaticHost,
    string? StaticIp,
    Guid? PulseAgentId,
    string PulseIpMode,
    string PrivateCandidateSelector,
    int Port,
    string Scheme,
    string? PathPrefix,
    string TlsValidationMode,
    string? ExpectedTlsHostname,
    string? ResolvedIpSnapshot,
    DateTimeOffset? LastResolvedAtUtc,
    string Status,
    string? LastError);

public sealed record ConnectionTargetRequest(
    string TargetMode,
    string? StaticHost,
    string? StaticIp,
    Guid? PulseAgentId,
    string? PulseIpMode,
    string? PrivateCandidateSelector,
    int Port,
    string Scheme,
    string? PathPrefix,
    string? TlsValidationMode,
    string? ExpectedTlsHostname);

public sealed record PulseResolvedTargetResponse(
    Guid PulseAgentId,
    string AgentName,
    string IpMode,
    string? SelectedIp,
    string? PublicIp,
    IReadOnlyList<string> PrivateIpv4Candidates,
    IReadOnlyList<string> PrivateIpv6Candidates,
    DateTimeOffset? LastSeenAtUtc,
    string Status,
    string? ResolvedIp = null,
    string? Error = null);

public sealed record InternalAgentDnsSettingsResponse(
    bool Enabled,
    string Domain,
    bool KeepLastRewriteWhenAgentStale,
    Guid? AdGuardConnectionId,
    string LastSyncStatus,
    string? LastAppliedHash,
    DateTimeOffset UpdatedAtUtc,
    IReadOnlyList<InternalAgentDnsAgentSettingsResponse> Agents);

public sealed record InternalAgentDnsAgentSettingsResponse(
    Guid Id,
    Guid PulseAgentId,
    bool Enabled,
    string? NameOverride,
    string IpMode,
    bool KeepLastRewriteWhenStale,
    DateTimeOffset UpdatedAtUtc);

public sealed record InternalAgentDnsSettingsRequest(
    bool Enabled,
    string? Domain,
    bool? KeepLastRewriteWhenAgentStale,
    Guid? AdGuardConnectionId,
    IReadOnlyList<InternalAgentDnsAgentSettingsRequest>? Agents);

public sealed record InternalAgentDnsAgentSettingsRequest(
    Guid PulseAgentId,
    bool Enabled,
    string? NameOverride,
    string? IpMode,
    bool? KeepLastRewriteWhenStale);

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
    string? PathPrefix = null,
    string? RequestId = null,
    string? UserAgent = null,
    string? UserAgentHash = null);

public sealed record WafEventIngestRequest(
    string ClientIp,
    string Host,
    string Path,
    string Action,
    string? RequestId = null,
    string? UserAgent = null,
    string? UserAgentHash = null);

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

public sealed record NotificationRouteResponse(
    Guid Id,
    Guid ProviderId,
    string Name,
    string EventKind,
    string Severity,
    string MatchJson,
    bool Enabled,
    int CooldownMinutes,
    bool SendRecovery,
    DateTimeOffset CreatedAtUtc);

public sealed record CreateNotificationRouteRequest(
    Guid ProviderId,
    string Name,
    string EventKind,
    string Severity,
    string MatchJson,
    bool Enabled,
    int CooldownMinutes,
    bool SendRecovery);

public sealed record UpdateNotificationRouteRequest(
    Guid? ProviderId,
    string? Name,
    string? EventKind,
    string? Severity,
    string? MatchJson,
    bool? Enabled,
    int? CooldownMinutes,
    bool? SendRecovery);

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
    string? Resource = null,
    string? RequestId = null,
    string? UserAgent = null,
    string? UserAgentHash = null);

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

public sealed record AdGuardConnectionResponse(
    Guid Id,
    string Name,
    string BaseUrl,
    bool Enabled,
    ConnectionTargetResponse? Target = null,
    string? ResolvedBaseUrl = null,
    string TargetStatus = "unresolved",
    string? TargetError = null);

public sealed record CreateAdGuardConnectionRequest(
    string Name,
    string? BaseUrl,
    string Password,
    ConnectionTargetRequest? Target = null);

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

public sealed record SecurityProfileResponse(
    string Name,
    string ForwardAuthPolicy,
    string WafMode,
    int RateLimitAverage,
    int RateLimitBurst);

public sealed record CreateSecurityProfileRequest(
    string Name,
    string ForwardAuthPolicy,
    string WafMode,
    int RateLimitAverage,
    int RateLimitBurst);

public sealed record UpdateSecurityProfileRequest(
    string ForwardAuthPolicy,
    string WafMode,
    int RateLimitAverage,
    int RateLimitBurst);

public sealed record AdminDashboardResponse(
    IReadOnlyList<AuditEventResponse> AuditEvents,
    HealthResponse Health,
    VaultStatusResponse Vault,
    IReadOnlyList<ResourceResponse> Resources,
    IReadOnlyList<MonitorEndpointResponse> Monitors,
    SecurityDashboardResponse Security,
    IReadOnlyList<ConnectionSummaryResponse> DnsConnections,
    IReadOnlyList<PulseAgentResponse> PulseAgents,
    IReadOnlyList<SyncRunResponse> SyncRuns);
