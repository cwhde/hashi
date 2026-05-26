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
        if (string.IsNullOrWhiteSpace(yaml))
        {
            errors.Add("Static config is empty.");
            return;
        }

        RequireContains(yaml, "entryPoints:", "Static config must define entryPoints.", errors);
        RequireContains(yaml, "providers:", "Static config must define providers.", errors);
        RequireContains(yaml, "directory: /etc/hashi/traefik/dynamic", "Static config must point at Hashi dynamic directory.", errors);
    }

    private static void ValidateDynamic(string name, string yaml, List<string> errors)
    {
        if (string.IsNullOrWhiteSpace(yaml))
        {
            errors.Add($"Dynamic file '{name}' is empty.");
            return;
        }

        if (!yaml.Contains("http:", StringComparison.Ordinal)
            && !yaml.Contains("tcp:", StringComparison.Ordinal)
            && !yaml.Contains("udp:", StringComparison.Ordinal))
        {
            errors.Add($"Dynamic file '{name}' must contain http, tcp, or udp section.");
        }
    }

    private static void ValidateUserMiddlewares(string yaml, List<string> errors)
    {
        var parsed = TraefikUserMiddlewareParser.Parse(yaml);
        if (!parsed.IsValid)
        {
            errors.Add(parsed.Error ?? "User middleware YAML is invalid.");
        }
    }

    private static void RequireContains(string haystack, string needle, string message, List<string> errors)
    {
        if (!haystack.Contains(needle, StringComparison.Ordinal))
        {
            errors.Add(message);
        }
    }
}
