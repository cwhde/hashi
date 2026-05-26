using System.Text;

namespace Hashi.Core.Traefik;

public sealed record TraefikUserMiddlewareParseResult(
    bool IsValid,
    IReadOnlyList<string> MiddlewareNames,
    string? Error,
    string NormalizedYaml);

public static class TraefikUserMiddlewareParser
{
    public const string DefaultYaml = """
        http:
          middlewares: {}
        """;

    public static TraefikUserMiddlewareParseResult Parse(string? yaml)
    {
        if (string.IsNullOrWhiteSpace(yaml))
        {
            return new(true, [], null, DefaultYaml);
        }

        var lines = yaml.Replace("\r\n", "\n", StringComparison.Ordinal).Split('\n');
        var httpIndex = -1;
        for (var i = 0; i < lines.Length; i++)
        {
            if (lines[i].TrimStart().StartsWith("http:", StringComparison.Ordinal))
            {
                httpIndex = i;
                break;
            }
        }

        if (httpIndex < 0)
        {
            return new(false, [], "YAML must contain a top-level http: section.", yaml);
        }

        var middlewaresIndex = -1;
        var httpIndent = LeadingSpaces(lines[httpIndex]);
        for (var i = httpIndex + 1; i < lines.Length; i++)
        {
            var trimmed = lines[i].Trim();
            if (trimmed.Length == 0 || trimmed.StartsWith('#'))
            {
                continue;
            }

            var indent = LeadingSpaces(lines[i]);
            if (indent <= httpIndent)
            {
                break;
            }

            if (trimmed.StartsWith("middlewares:", StringComparison.Ordinal))
            {
                middlewaresIndex = i;
                break;
            }
        }

        if (middlewaresIndex < 0)
        {
            return new(false, [], "YAML must define http.middlewares.", yaml);
        }

        var middlewaresLine = lines[middlewaresIndex].Trim();
        if (middlewaresLine.Equals("middlewares: {}", StringComparison.Ordinal)
            || middlewaresLine.Equals("middlewares:{}", StringComparison.Ordinal))
        {
            return new(true, [], null, NormalizeYaml(yaml));
        }

        var middlewareIndent = LeadingSpaces(lines[middlewaresIndex]);
        var names = new List<string>();
        for (var i = middlewaresIndex + 1; i < lines.Length; i++)
        {
            var trimmed = lines[i].Trim();
            if (trimmed.Length == 0 || trimmed.StartsWith('#'))
            {
                continue;
            }

            var indent = LeadingSpaces(lines[i]);
            if (indent <= middlewareIndent)
            {
                break;
            }

            if (indent != middlewareIndent + 2)
            {
                continue;
            }

            var name = trimmed.Split(':')[0].Trim();
            if (name.Length == 0)
            {
                return new(false, [], $"Invalid middleware name near line {i + 1}.", yaml);
            }

            if (names.Contains(name, StringComparer.Ordinal))
            {
                return new(false, [], $"Duplicate middleware name '{name}'.", yaml);
            }

            names.Add(name);
        }

        if (names.Count == 0)
        {
            return new(false, [], "http.middlewares must define at least one middleware or use {}.", yaml);
        }

        return new(true, names, null, NormalizeYaml(yaml));
    }

    public static string NormalizeYaml(string yaml)
    {
        var normalized = yaml.Replace("\r\n", "\n", StringComparison.Ordinal).TrimEnd();
        return normalized.Length == 0 ? DefaultYaml : normalized + "\n";
    }

    private static int LeadingSpaces(string line)
    {
        var count = 0;
        foreach (var ch in line)
        {
            if (ch == ' ')
            {
                count++;
            }
            else
            {
                break;
            }
        }

        return count;
    }
}
