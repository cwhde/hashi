using System.IO;
using YamlDotNet.Core;
using YamlDotNet.RepresentationModel;

namespace Hashi.Core.Traefik;

public sealed record TraefikConfigValidationResult(bool IsValid, IReadOnlyList<string> Errors);

public static class TraefikConfigValidator
{
    public static TraefikConfigValidationResult ValidateRender(TraefikRenderResult render)
    {
        var errors = new List<string>();
        ValidateStatic(render.StaticConfigYaml, errors);
        ValidateDynamic("core", render.DynamicFiles.CoreYaml, errors);
        ValidateDynamic("http-resources", render.DynamicFiles.HttpResourcesYaml, errors);
        ValidateDynamic("stream-resources", render.DynamicFiles.StreamResourcesYaml, errors);
        ValidateUserMiddlewares(render.DynamicFiles.UserMiddlewaresYaml, errors);
        ValidateDynamic("security", render.DynamicFiles.SecurityYaml, errors);
        ValidateDynamic("health", render.DynamicFiles.HealthYaml, errors);

        if (string.IsNullOrWhiteSpace(render.ContentHash))
        {
            errors.Add("Content hash is missing.");
        }

        return new TraefikConfigValidationResult(errors.Count == 0, errors);
    }

    private static void ValidateStatic(string yaml, List<string> errors)
    {
        if (!TryParseYaml("Static config", yaml, errors, out var root))
        {
            return;
        }

        RequireMapping(root, "entryPoints", "Static config must define entryPoints.", errors);
        if (!YamlNavigationHelpers.TryGetMapping(root, "providers", out var providers))
        {
            errors.Add("Static config must define providers.");
            return;
        }

        if (!YamlNavigationHelpers.TryGetMapping(providers, "file", out var fileProvider)
            || !YamlNavigationHelpers.TryGetScalar(fileProvider, "directory", out var directory)
            || !string.Equals(directory, "/etc/hashi/traefik/dynamic", StringComparison.Ordinal))
        {
            errors.Add("Static config must point at Hashi dynamic directory.");
        }
    }

    private static void ValidateDynamic(string name, string yaml, List<string> errors)
    {
        if (!TryParseYaml($"Dynamic file '{name}'", yaml, errors, out var root))
        {
            return;
        }

        if (!HasKey(root, "http") && !HasKey(root, "tcp") && !HasKey(root, "udp"))
        {
            errors.Add($"Dynamic file '{name}' must contain http, tcp, or udp section.");
        }

        ValidateReplacePathRegex(name, root, errors);
    }

    private static void ValidateUserMiddlewares(string yaml, List<string> errors)
    {
        var parsed = TraefikUserMiddlewareParser.Parse(yaml);
        if (!parsed.IsValid)
        {
            errors.Add(parsed.Error ?? "User middleware YAML is invalid.");
        }
    }

    private static bool TryParseYaml(string name, string yaml, List<string> errors, out YamlMappingNode root)
    {
        root = null!;
        if (string.IsNullOrWhiteSpace(yaml))
        {
            errors.Add($"{name} is empty.");
            return false;
        }

        try
        {
            var stream = new YamlStream();
            stream.Load(new StringReader(yaml));
            if (stream.Documents.Count == 0 || stream.Documents[0].RootNode is not YamlMappingNode mapping)
            {
                errors.Add($"{name} must be a YAML mapping.");
                return false;
            }

            root = mapping;
            return true;
        }
        catch (Exception ex) when (ex is YamlException or ArgumentException or InvalidOperationException)
        {
            errors.Add($"{name} YAML parse error: {ex.Message}");
            return false;
        }
    }

    private static void ValidateReplacePathRegex(string name, YamlMappingNode root, List<string> errors)
    {
        if (!YamlNavigationHelpers.TryGetMapping(root, "http", out var http)
            || !YamlNavigationHelpers.TryGetMapping(http, "middlewares", out var middlewares))
        {
            return;
        }

        foreach (var (middlewareKey, middlewareNode) in middlewares.Children)
        {
            if (middlewareNode is not YamlMappingNode middleware
                || !YamlNavigationHelpers.TryGetMapping(middleware, "replacePathRegex", out var replacePathRegex))
            {
                continue;
            }

            var middlewareName = middlewareKey is YamlScalarNode scalar ? scalar.Value : middlewareKey.ToString();
            if (!YamlNavigationHelpers.TryGetScalar(replacePathRegex, "regex", out var regex) || string.IsNullOrWhiteSpace(regex))
            {
                errors.Add($"Dynamic file '{name}' middleware '{middlewareName}' replacePathRegex must define regex.");
            }

            if (!YamlNavigationHelpers.TryGetScalar(replacePathRegex, "replacement", out var replacement) || string.IsNullOrWhiteSpace(replacement))
            {
                errors.Add($"Dynamic file '{name}' middleware '{middlewareName}' replacePathRegex must define replacement.");
            }
        }
    }

    private static void RequireMapping(YamlMappingNode node, string key, string message, List<string> errors)
    {
        if (!YamlNavigationHelpers.TryGetMapping(node, key, out _))
        {
            errors.Add(message);
        }
    }

    private static bool HasKey(YamlMappingNode node, string key)
        => YamlNavigationHelpers.TryGetNode(node, key, out _);
}
