namespace Hashi.Core.Resources;

public enum ResourceKind
{
    Http,
    Https,
    H2c,
    Tcp,
    Udp,
}

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
    string? PathPrefix = null,
    string? PathRewrite = null,
    ForwardAuthPolicy ForwardAuth = ForwardAuthPolicy.Adaptive,
    Hashi.Core.Security.WafMode WafMode = Hashi.Core.Security.WafMode.DetectOnly);

public static class ResourceSlug
{
    public static string Normalize(string name)
        => new string(name.Trim().ToLowerInvariant()
            .Select(ch => char.IsLetterOrDigit(ch) ? ch : '-')
            .ToArray())
            .Trim('-');
}
