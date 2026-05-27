using System.IO;
using YamlDotNet.RepresentationModel;

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

        YamlStream stream;
        try
        {
            stream = LoadYaml(yaml);
        }
        catch (Exception ex) when (ex is YamlDotNet.Core.YamlException or ArgumentException or InvalidOperationException)
        {
            return new(false, [], $"YAML parse error: {ex.Message}", yaml);
        }

        if (stream.Documents.Count == 0 || stream.Documents[0].RootNode is not YamlMappingNode root)
        {
            return new(false, [], "YAML must be a mapping with a top-level http: section.", yaml);
        }

        if (!TryGetMapping(root, "http", out var http))
        {
            return new(false, [], "YAML must contain a top-level http: section.", yaml);
        }

        if (!TryGetNode(http, "middlewares", out var middlewaresNode))
        {
            return new(false, [], "YAML must define http.middlewares.", yaml);
        }

        if (middlewaresNode is not YamlMappingNode middlewares)
        {
            return new(false, [], "http.middlewares must be a YAML mapping.", yaml);
        }

        var names = middlewares.Children.Keys
            .OfType<YamlScalarNode>()
            .Select(x => x.Value)
            .Where(x => !string.IsNullOrWhiteSpace(x))
            .Cast<string>()
            .ToList();

        return new(true, names, null, NormalizeYaml(yaml));
    }

    public static string NormalizeYaml(string yaml)
    {
        var normalized = yaml.Replace("\r\n", "\n", StringComparison.Ordinal).TrimEnd();
        return normalized.Length == 0 ? DefaultYaml : normalized + "\n";
    }

    private static YamlStream LoadYaml(string yaml)
    {
        var stream = new YamlStream();
        stream.Load(new StringReader(yaml));
        return stream;
    }

    private static bool TryGetMapping(YamlMappingNode node, string key, out YamlMappingNode mapping)
    {
        if (TryGetNode(node, key, out var value) && value is YamlMappingNode child)
        {
            mapping = child;
            return true;
        }

        mapping = null!;
        return false;
    }

    private static bool TryGetNode(YamlMappingNode node, string key, out YamlNode value)
    {
        foreach (var (candidateKey, candidateValue) in node.Children)
        {
            if (candidateKey is YamlScalarNode scalar && string.Equals(scalar.Value, key, StringComparison.Ordinal))
            {
                value = candidateValue;
                return true;
            }
        }

        value = null!;
        return false;
    }
}
