using Hashi.Core.Resources;
using Hashi.Core.Security;
using Hashi.Core.Traefik;
using Xunit;

namespace Hashi.UnitTests;

public sealed class TraefikConfigRendererTests
{
    [Fact]
    public void Render_produces_stable_hash_for_same_input()
    {
        IReadOnlyList<ResourceDefinition> resources =
        [
            new ResourceDefinition(Guid.NewGuid(), "Hashi", "hashi", ResourceKind.Https, true, true, "hashi.example.com", "http", "127.0.0.1", 8080),
        ];
        var first = TraefikConfigRenderer.Render(resources);
        var second = TraefikConfigRenderer.Render(resources);
        Assert.Equal(first.ContentHash, second.ContentHash);
        Assert.Contains("hashi.example.com", second.DynamicFiles.HttpResourcesYaml);
    }

    [Fact]
    public void Render_includes_acme_and_coraza_when_configured()
    {
        var resources = new List<ResourceDefinition>
        {
            new(Guid.NewGuid(), "App", "app", ResourceKind.Https, true, false, "app.example.com", "http", "10.0.0.2", 8080,
                ForwardAuthPolicy.Adaptive, WafMode.On),
        };
        var options = new TraefikRenderOptions(
            AcmeEmail: "admin@example.com",
            AcmeEabKeyId: "eab-key",
            AcmeEabHmac: "eab-hmac");
        var result = TraefikConfigRenderer.Render(resources, options);

        Assert.Contains("certificatesResolvers:", result.StaticConfigYaml);
        Assert.Contains("externalAccountBinding:", result.StaticConfigYaml);
        Assert.Contains("coraza:", result.StaticConfigYaml);
        Assert.Contains("app-waf:", result.DynamicFiles.SecurityYaml);
        Assert.Contains("hashi-forward-auth", result.DynamicFiles.HttpResourcesYaml);
    }

    [Fact]
    public void Render_omits_forward_auth_when_policy_off()
    {
        var resources = new List<ResourceDefinition>
        {
            new(Guid.NewGuid(), "Public", "public", ResourceKind.Http, true, false, "public.example.com", "http", "10.0.0.2", 8080,
                ForwardAuthPolicy.Off, WafMode.Off),
        };
        var result = TraefikConfigRenderer.Render(resources);

        Assert.DoesNotContain("hashi-forward-auth", result.DynamicFiles.HttpResourcesYaml);
        Assert.DoesNotContain("public-waf", result.DynamicFiles.SecurityYaml);
    }

    [Fact]
    public void Render_uses_strict_forward_auth_for_sso_required()
    {
        var resources = new List<ResourceDefinition>
        {
            new(Guid.NewGuid(), "Secure", "secure", ResourceKind.Https, true, false, "secure.example.com", "http", "10.0.0.2", 8080,
                ForwardAuthPolicy.SsoRequired, WafMode.DetectOnly),
        };
        var result = TraefikConfigRenderer.Render(resources);

        Assert.Contains("hashi-forward-auth-strict", result.DynamicFiles.HttpResourcesYaml);
    }
}

public sealed class ResourceSlugTests
{
    [Fact]
    public void Normalize_replaces_invalid_characters()
    {
        Assert.Equal("my-app", ResourceSlug.Normalize("My App!"));
    }
}
