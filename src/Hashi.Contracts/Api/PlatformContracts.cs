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
    bool StatusEnabled);

public sealed record CreateResourceRequest(
    string Name,
    string Kind,
    string? Domain,
    string TargetScheme,
    string TargetHost,
    int TargetPort,
    bool DashboardEnabled,
    bool StatusEnabled);

public sealed record UpdateResourceRequest(
    string? Name,
    bool? Enabled,
    string? Domain,
    string? TargetScheme,
    string? TargetHost,
    int? TargetPort,
    bool? DashboardEnabled,
    bool? StatusEnabled);

public sealed record TraefikRenderResponse(string StaticConfigYaml, string DynamicHttpYaml, string ContentHash);

public sealed record FirewallRenderRequest(string Name, string Domain, IReadOnlyList<string> ManagedSubnets, string LinkedTraefikHost, string InternalTraefikIp);

public sealed record FirewallRenderResponse(string Script);

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
