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
    bool DashboardEnabled,
    bool StatusEnabled,
    Guid? FirewallHostId);

public sealed record CreateResourceRequest(
    string Name,
    string Kind,
    string? Domain,
    string TargetScheme,
    string TargetHost,
    int TargetPort,
    bool DashboardEnabled,
    bool StatusEnabled,
    Guid? FirewallHostId = null);

public sealed record UpdateResourceRequest(
    string? Name,
    bool? Enabled,
    string? Domain,
    string? TargetScheme,
    string? TargetHost,
    int? TargetPort,
    bool? DashboardEnabled,
    bool? StatusEnabled,
    Guid? FirewallHostId = null,
    bool ClearFirewallHostId = false);

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

public sealed record FirewallRenderRequest(string Name, string Domain, IReadOnlyList<string> ManagedSubnets, string LinkedTraefikHost, string InternalTraefikIp);

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
    IReadOnlyList<string> ManagedSubnets,
    bool NetBirdDetected,
    DateTimeOffset? LastAppliedAtUtc);

public sealed record CreateFirewallHostRequest(
    Guid ConnectionId,
    string Name,
    string Domain,
    IReadOnlyList<string> ManagedSubnets,
    string LinkedTraefikHost,
    string InternalTraefikIp,
    string? PublicIp = null);

public sealed record MonitorRollupResponse(
    Guid MonitorEndpointId,
    DateTimeOffset BucketStartUtc,
    int SampleCount,
    int UpCount,
    int DownCount,
    double AverageLatencyMs);

public sealed record MonitorEndpointResponse(
    Guid Id,
    string Name,
    string Url,
    string CheckType,
    bool Enabled,
    string Status,
    DateTimeOffset? LastCheckedAtUtc,
    int? LastLatencyMs);

public sealed record PublicStatusItemResponse(string Name, string Status, int? LastLatencyMs);

public sealed record PulseAgentResponse(Guid Id, string Name, string Status, DateTimeOffset? LastSeenAtUtc, string? LastPublicIp);

public sealed record PulseHeartbeatRequest(string Version, string Hostname, IReadOnlyList<string> PrivateIpv4Candidates);

public sealed record EdgeAuthForwardResponse(string Decision, string? RedirectUrl);

public sealed record SecurityDashboardResponse(long Allowed, long Blocked, long Challenged, IReadOnlyList<string> TopBlockedIps);

public sealed record ScriptResponse(Guid Id, string Name, bool Enabled, string Description);

public sealed record NotificationProviderResponse(Guid Id, string Name, string Type, bool Enabled);

public sealed record CreateNotificationProviderRequest(string Name, string Type, string SettingsJson, bool Enabled);

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

public sealed record CreateScriptRequest(Guid ConnectionId, string Name, string Description, string Body, string CronExpression);

public sealed record RunScriptRequest(
    string Host,
    int Port,
    string Username,
    string AuthMode,
    string? Password,
    string? PrivateKeyPem,
    string? PrivateKeyPassphrase);

public sealed record RunScriptResponse(bool Succeeded, string Output, string? Error);

public sealed record CreatePulseAgentRequest(string Name);

public sealed record CreatePulseAgentResponse(Guid Id, string Name, string Token);

public sealed record PulseHeartbeatAuthRequest(string Token, string Version, string Hostname, IReadOnlyList<string> PrivateIpv4Candidates);
