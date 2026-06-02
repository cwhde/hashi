namespace Hashi.Core.Resources;

public enum ResourceKind
{
    Http,
    Https,
    H2c,
    Tcp,
    Udp,
}

public sealed record ResourceRouteDefinition(
    int Priority,
    string PathMatchType,
    string PathValue,
    string TargetScheme,
    string TargetHost,
    int TargetPort,
    bool Enabled = true,
    string? RewriteMode = null,
    string? RewriteValue = null,
    IReadOnlyList<string>? ExtraMiddlewares = null);

public sealed record ResourceRuleDefinition(
    int Priority,
    string Action,
    string MatchType,
    string MatchValue,
    bool Enabled = true);

public sealed record ResourceDefinition(
    Guid Id,
    string Name,
    string Slug,
    ResourceKind Kind,
    bool Enabled,
    bool IsSystem,
    string? Domain,
    string TargetScheme,
    string TargetHost,
    int TargetPort,
    int? PublicPort = null,
    string? PathPrefix = null,
    string? PathRewrite = null,
    ForwardAuthPolicy ForwardAuth = ForwardAuthPolicy.Adaptive,
    Hashi.Core.Security.WafMode WafMode = Hashi.Core.Security.WafMode.DetectOnly,
    IReadOnlyList<string>? ExtraMiddlewares = null,
    IReadOnlyList<ResourceRouteDefinition>? Routes = null,
    IReadOnlyList<ResourceRuleDefinition>? Rules = null,
    IReadOnlyList<string>? WafExclusions = null)
{
    public int EffectivePublicPort => PublicPort ?? TargetPort;
}

public static class ResourceSlug
{
    public static string Normalize(string name)
        => new string(name.Trim().ToLowerInvariant()
            .Select(ch => char.IsLetterOrDigit(ch) ? ch : '-')
            .ToArray())
            .Trim('-');
}
