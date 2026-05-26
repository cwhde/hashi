using Hashi.Core.Resources;
using Hashi.Core.Security;
using Hashi.Core.Traefik;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Platform;
using System.Text.Json;
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
                PathPrefix: null, PathRewrite: null, ForwardAuthPolicy.Adaptive, WafMode.On),
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
                PathPrefix: null, PathRewrite: null, ForwardAuthPolicy.Off, WafMode.Off),
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
                PathPrefix: null, PathRewrite: null, ForwardAuthPolicy.SsoRequired, WafMode.DetectOnly),
        };
        var result = TraefikConfigRenderer.Render(resources);

        Assert.Contains("hashi-forward-auth-strict", result.DynamicFiles.HttpResourcesYaml);
    }
}

public sealed class MonitorRollupServiceTests
{
    [Fact]
    public void FloorToBucket_aligns_to_interval_start()
    {
        var time = new DateTimeOffset(2026, 5, 26, 8, 27, 45, TimeSpan.Zero);
        var bucket = MonitorRollupService.FloorToBucket(time, 5);
        Assert.Equal(new DateTimeOffset(2026, 5, 26, 8, 25, 0, TimeSpan.Zero), bucket);
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

public sealed class FirewallHostResponseTests
{
    [Fact]
    public void ToResponse_maps_managed_subnets_and_metadata()
    {
        var hostId = Guid.NewGuid();
        var connectionId = Guid.NewGuid();
        var appliedAt = DateTimeOffset.UtcNow;
        var host = new FirewallHostEntity
        {
            Id = hostId,
            ConnectionId = connectionId,
            Name = "edge-1",
            Domain = "edge.example.com",
            ManagedSubnetsJson = JsonSerializer.Serialize(new[] { "10.0.0.0/24", "192.168.1.0/24" }),
            LinkedTraefikHost = "traefik.internal",
            InternalTraefikIp = "10.0.0.2",
            PublicIp = "203.0.113.10",
            NetBirdDetected = true,
            LastAppliedAtUtc = appliedAt,
        };

        var response = FirewallApplyService.ToResponse(host);

        Assert.Equal(hostId, response.Id);
        Assert.Equal(connectionId, response.ConnectionId);
        Assert.Equal("edge-1", response.Name);
        Assert.Equal("edge.example.com", response.Domain);
        Assert.Equal("203.0.113.10", response.PublicIp);
        Assert.Equal(["10.0.0.0/24", "192.168.1.0/24"], response.ManagedSubnets);
        Assert.True(response.NetBirdDetected);
        Assert.Equal(appliedAt, response.LastAppliedAtUtc);
    }
}
