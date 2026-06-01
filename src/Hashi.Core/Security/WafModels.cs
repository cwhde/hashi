namespace Hashi.Core.Security;

public enum WafMode
{
    Off,
    DetectOnly,
    On,
}

public sealed record WafMiddlewareDefinition(
    string Slug,
    WafMode Mode,
    IReadOnlyList<string>? Exclusions = null);

public static class WafMiddlewareRenderer
{
    public static string RenderCorazaMiddleware(
        string slug,
        WafMode mode,
        IReadOnlyList<string>? exclusions = null)
        => RenderCorazaMiddlewares([new WafMiddlewareDefinition(slug, mode, exclusions)]);

    public static string RenderCorazaMiddlewares(IReadOnlyList<WafMiddlewareDefinition> middlewares)
    {
        var enabled = middlewares.Where(x => x.Mode != WafMode.Off).ToList();
        if (enabled.Count == 0)
        {
            return "http:\n  middlewares: {}\n";
        }

        return "http:\n  middlewares:\n"
               + string.Join('\n', enabled.Select(RenderCorazaMiddlewareEntry))
               + "\n";
    }

    public static string RenderCorazaMiddlewareEntry(WafMiddlewareDefinition middleware)
    {
        var modeDirective = middleware.Mode switch
        {
            WafMode.Off => "SecRuleEngine Off",
            WafMode.DetectOnly => "SecRuleEngine DetectionOnly",
            _ => "SecRuleEngine On",
        };

        var directives = new List<string> { modeDirective };
        directives.AddRange((middleware.Exclusions ?? [])
            .Where(x => !string.IsNullOrWhiteSpace(x))
            .Select(x => x.Trim()));
        directives.AddRange([
            "Include @owasp_crs/REQUEST-900-EXCLUSION-RULES-BEFORE-CRS.conf",
            "Include @owasp_crs/REQUEST-941-APPLICATION-ATTACK-XSS.conf",
            "Include @owasp_crs/RESPONSE-959-BLOCKING-EVALUATION.conf",
        ]);
        var directiveLines = string.Join('\n', directives.Select(x => $"          - \"{EscapeYamlDoubleQuoted(x)}\""));

        return $$"""
              {{middleware.Slug}}-waf:
                plugin:
                  coraza:
                    directives:
            {{directiveLines}}
            """;
    }

    private static string EscapeYamlDoubleQuoted(string value)
        => value
            .Replace("\\", "\\\\", StringComparison.Ordinal)
            .Replace("\"", "\\\"", StringComparison.Ordinal)
            .Replace("\r", "\\r", StringComparison.Ordinal)
            .Replace("\n", "\\n", StringComparison.Ordinal)
            .Replace("\t", "\\t", StringComparison.Ordinal);
}
