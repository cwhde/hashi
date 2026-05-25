namespace Hashi.Core.Traefik;

public sealed record TraefikRenderResult(
    string StaticConfigYaml,
    string DynamicHttpYaml,
    string ContentHash);

public static class TraefikConfigRenderer
{
    public static TraefikRenderResult Render(IReadOnlyList<Hashi.Core.Resources.ResourceDefinition> resources)
    {
        var staticYaml = """
            entryPoints:
              web:
                address: ":80"
              websecure:
                address: ":443"
            providers:
              file:
                directory: /etc/hashi/traefik/dynamic
                watch: true
            """;
        var routers = string.Join('\n', resources.Where(r => r.Enabled).Select(r =>
            $"  {r.Slug}:\n    rule: Host(`{r.Domain}`)\n    service: {r.Slug}"));
        var services = string.Join('\n', resources.Where(r => r.Enabled).Select(r =>
            $"  {r.Slug}:\n    loadBalancer:\n      servers:\n        - url: \"{r.TargetScheme}://{r.TargetHost}:{r.TargetPort}\""));
        var dynamicYaml = $"http:\n  routers:\n{routers}\n  services:\n{services}\n";
        var hash = Convert.ToHexString(System.Security.Cryptography.SHA256.HashData(
            System.Text.Encoding.UTF8.GetBytes(staticYaml + dynamicYaml))).ToLowerInvariant();
        return new TraefikRenderResult(staticYaml, dynamicYaml, hash);
    }
}
