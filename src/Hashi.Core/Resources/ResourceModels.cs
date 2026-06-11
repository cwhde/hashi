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

public static class ResourceRuleActionNames
{
    public const string Allow = "allow";
    public const string Deny = "deny";
    public const string RequireSso = "require_sso";
    public const string RequireChallenge = "require_challenge";
    public const string SoftBlock = "soft_block";
    public const string FirewallBlock = "firewall_block";
    public const string BypassBlocking = "bypass_blocking";
    public const string BypassAuth = "bypass_auth";
    public const string BlockAccess = "block_access";
    public const string PassToAuth = "pass_to_auth";
    public const string RequireAdaptiveChallenge = "require_adaptive_challenge";

    public static readonly IReadOnlySet<string> All = new HashSet<string>(StringComparer.OrdinalIgnoreCase)
    {
        Allow,
        Deny,
        RequireSso,
        RequireChallenge,
        SoftBlock,
        FirewallBlock,
        BypassBlocking,
    };

    private static readonly IReadOnlyDictionary<string, string> Aliases = new Dictionary<string, string>(StringComparer.OrdinalIgnoreCase)
    {
        [Allow] = Allow,
        [BypassAuth] = Allow,
        ["bypass"] = Allow,
        [Deny] = Deny,
        [BlockAccess] = Deny,
        ["block"] = Deny,
        [RequireSso] = RequireSso,
        [PassToAuth] = RequireSso,
        ["auth"] = RequireSso,
        ["require_auth"] = RequireSso,
        [RequireChallenge] = RequireChallenge,
        [RequireAdaptiveChallenge] = RequireChallenge,
        ["challenge"] = RequireChallenge,
        ["adaptive_challenge"] = RequireChallenge,
        [SoftBlock] = SoftBlock,
        ["soft-block"] = SoftBlock,
        [FirewallBlock] = FirewallBlock,
        ["firewall-block"] = FirewallBlock,
        [BypassBlocking] = BypassBlocking,
    };

    public static bool TryNormalize(string? action, out string normalized)
    {
        normalized = string.Empty;
        if (string.IsNullOrWhiteSpace(action))
        {
            return false;
        }

        if (!Aliases.TryGetValue(action.Trim().ToLowerInvariant(), out var value))
        {
            return false;
        }

        normalized = value;
        return true;
    }

    public static string Normalize(string? action)
        => TryNormalize(action, out var normalized)
            ? normalized
            : throw new InvalidOperationException($"Resource rule action must be one of: {string.Join(", ", All)}.");

    public static bool IsValid(string? action)
        => TryNormalize(action, out _);
}

public static class ResourceRuleMatchTypeNames
{
    public const string Ip = "ip";
    public const string Cidr = "cidr";
    public const string Path = "path";
    public const string Country = "country";
    public const string Region = "region";
    public const string Asn = "asn";

    public static readonly IReadOnlySet<string> All = new HashSet<string>(StringComparer.OrdinalIgnoreCase)
    {
        Ip,
        Cidr,
        Path,
        Country,
        Region,
        Asn,
    };

    public static bool IsValid(string? matchType)
        => !string.IsNullOrWhiteSpace(matchType) && All.Contains(matchType.Trim());
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
    string? MonitoringProtocolHint = null,
    bool ErrorHandlingEnabled = true,
    bool AdGuardRewriteEnabled = true,
    string? ExplicitRoutingOverride = null,
    string? SecurityProfileName = null,
    int? RateLimitAverage = null,
    int? RateLimitBurst = null)
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
    private const int MaxLength = 63;

    public static string Normalize(string name)
    {
        ArgumentException.ThrowIfNullOrWhiteSpace(name);

        var result = new string(name.Trim().ToLowerInvariant()
            .Select(ch => char.IsLetterOrDigit(ch) ? ch : '-')
            .ToArray());

        result = System.Text.RegularExpressions.Regex.Replace(result, @"-+", "-");
        result = result.Trim('-');

        if (result.Length == 0)
        {
            throw new ArgumentException("Resource name must contain at least one letter or digit.", nameof(name));
        }

        if (result.Length > MaxLength)
        {
            result = result[..MaxLength].TrimEnd('-');
        }

        return result;
    }
}
