namespace Hashi.Core.Resources;

public enum ResourceKind
{
    Http,
    Https,
    H2c,
    Tcp,
    Udp,
}

public static class ResourceDomainModeNames
{
    public const string Root = "root";
    public const string Subdomain = "subdomain";
    public const string Custom = "custom";

    public static readonly IReadOnlySet<string> All = new HashSet<string>(StringComparer.OrdinalIgnoreCase)
    {
        Root,
        Subdomain,
        Custom,
    };

    public static bool IsValid(string? mode)
        => !string.IsNullOrWhiteSpace(mode) && All.Contains(mode.Trim());
}

public static class ResourceRewriteModeNames
{
    public const string ReplacePath = "replace_path";
    public const string ReplacePrefix = "replace_prefix";
    public const string StripPrefix = "strip_prefix";
    public const string Regex = "regex";

    public static readonly IReadOnlySet<string> All = new HashSet<string>(StringComparer.OrdinalIgnoreCase)
    {
        ReplacePath,
        ReplacePrefix,
        StripPrefix,
        Regex,
    };

    public static bool IsValid(string? mode)
        => !string.IsNullOrWhiteSpace(mode) && All.Contains(mode.Trim());
}

public static class ResourceMonitoringProtocolHintNames
{
    public const string Http = "http";
    public const string Https = "https";
    public const string H2c = "h2c";
    public const string Tcp = "tcp";
    public const string Udp = "udp";
    public const string Dns = "dns";
    public const string Icmp = "icmp";
    public const string Tls = "tls";
    public const string Pulse = "pulse";

    public static readonly IReadOnlySet<string> All = new HashSet<string>(StringComparer.OrdinalIgnoreCase)
    {
        Http,
        Https,
        H2c,
        Tcp,
        Udp,
        Dns,
        Icmp,
        Tls,
        Pulse,
    };

    public static bool IsValid(string? hint)
    {
        var normalized = Normalize(hint);
        return normalized is not null && All.Contains(normalized);
    }

    public static string? Normalize(string? hint)
    {
        if (string.IsNullOrWhiteSpace(hint))
        {
            return null;
        }

        var normalized = hint.Trim().ToLowerInvariant();
        return normalized == "push" ? Pulse : normalized;
    }
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
    IReadOnlyList<string>? WafExclusions = null,
    string DomainMode = ResourceDomainModeNames.Custom,
    string? PathRewriteMode = null,
    bool? TcpProxyProtocolEnabled = null,
    string? MonitoringProtocolHint = null)
{
    public int EffectivePublicPort => PublicPort ?? TargetPort;
}

public static class ResourceDomainResolver
{
    public static string? Resolve(string? domainMode, string? domain, string slug, string? rootDomain)
    {
        var mode = NormalizeMode(domainMode);
        var normalizedDomain = NormalizeDomain(domain);
        var normalizedRoot = NormalizeDomain(rootDomain);
        return mode switch
        {
            ResourceDomainModeNames.Root => normalizedRoot,
            ResourceDomainModeNames.Subdomain => ResolveSubdomain(normalizedDomain, slug, normalizedRoot),
            ResourceDomainModeNames.Custom => normalizedDomain == "@" ? normalizedRoot : normalizedDomain,
            _ => null,
        };
    }

    public static string NormalizeMode(string? domainMode)
        => string.IsNullOrWhiteSpace(domainMode)
            ? ResourceDomainModeNames.Custom
            : domainMode.Trim().ToLowerInvariant();

    public static string? NormalizeDomain(string? domain)
    {
        var normalized = domain?.Trim().TrimEnd('.');
        return string.IsNullOrWhiteSpace(normalized) ? null : normalized.ToLowerInvariant();
    }

    private static string? ResolveSubdomain(string? domain, string slug, string? rootDomain)
    {
        if (string.IsNullOrWhiteSpace(rootDomain))
        {
            return null;
        }

        var label = string.IsNullOrWhiteSpace(domain)
            ? slug
            : domain;
        return string.IsNullOrWhiteSpace(label)
            ? null
            : $"{label.Trim().TrimEnd('.')}.{rootDomain}".ToLowerInvariant();
    }
}

public static class ResourceSlug
{
    public static string Normalize(string name)
        => new string(name.Trim().ToLowerInvariant()
            .Select(ch => char.IsLetterOrDigit(ch) ? ch : '-')
            .ToArray())
            .Trim('-');
}
