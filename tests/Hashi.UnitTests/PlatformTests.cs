using Hashi.Core.Resources;
using Hashi.Core.Security;
using Hashi.Core.Traefik;
using Hashi.Core.Hosting;
using Hashi.Core.Firewall;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Platform;
using Hashi.UnitTests.Fakes;
using Microsoft.EntityFrameworkCore;
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
                ForwardAuth: ForwardAuthPolicy.Adaptive, WafMode: WafMode.On),
        };
        var options = new TraefikRenderOptions(
            AcmeEmail: "admin@example.com",
            AcmeEabKeyId: "eab-key",
            AcmeEabHmac: "eab-hmac",
            DnsProviderName: "hetzner");
        var result = TraefikConfigRenderer.Render(resources, options);

        Assert.Contains("certificatesResolvers:", result.StaticConfigYaml);
        Assert.Contains("provider: hetzner", result.StaticConfigYaml);
        Assert.Contains("externalAccountBinding:", result.StaticConfigYaml);
        Assert.Contains("coraza:", result.StaticConfigYaml);
        Assert.Contains("app-waf:", result.DynamicFiles.SecurityYaml);
        Assert.Contains("hashi-forward-auth", result.DynamicFiles.HttpResourcesYaml);
    }

    [Fact]
    public void Render_multiple_waf_resources_uses_single_security_http_map_and_validates()
    {
        var resources = new List<ResourceDefinition>
        {
            new(Guid.NewGuid(), "App", "app", ResourceKind.Https, true, false, "app.example.com", "http", "10.0.0.2", 8080,
                WafMode: WafMode.On, WafExclusions: ["SecRuleRemoveById 941100"]),
            new(Guid.NewGuid(), "Admin", "admin", ResourceKind.Https, true, false, "admin.example.com", "http", "10.0.0.3", 8080,
                WafMode: WafMode.DetectOnly, WafExclusions: ["SecRuleUpdateTargetById 942100 !ARGS:search"]),
        };

        var result = TraefikConfigRenderer.Render(resources);
        var validation = TraefikConfigValidator.ValidateRender(result);

        Assert.Equal(1, result.DynamicFiles.SecurityYaml.Split('\n').Count(line => line == "http:"));
        Assert.Contains("app-waf:", result.DynamicFiles.SecurityYaml);
        Assert.Contains("admin-waf:", result.DynamicFiles.SecurityYaml);
        Assert.Contains("SecRuleRemoveById 941100", result.DynamicFiles.SecurityYaml);
        Assert.Contains("SecRuleUpdateTargetById 942100 !ARGS:search", result.DynamicFiles.SecurityYaml);
        Assert.True(validation.IsValid, string.Join("; ", validation.Errors));
    }

    [Fact]
    public void Render_omits_forward_auth_when_policy_off()
    {
        var resources = new List<ResourceDefinition>
        {
            new(Guid.NewGuid(), "Public", "public", ResourceKind.Http, true, false, "public.example.com", "http", "10.0.0.2", 8080,
                ForwardAuth: ForwardAuthPolicy.Off, WafMode: WafMode.Off),
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
                ForwardAuth: ForwardAuthPolicy.SsoRequired, WafMode: WafMode.DetectOnly),
        };
        var result = TraefikConfigRenderer.Render(resources);

        Assert.Contains("hashi-forward-auth-strict", result.DynamicFiles.HttpResourcesYaml);
    }

    [Fact]
    public void Render_uses_configured_internal_urls_for_hashi_middlewares_and_health_service()
    {
        var resources = new List<ResourceDefinition>
        {
            new(Guid.NewGuid(), "App", "app", ResourceKind.Http, true, false, "app.example.com", "http", "10.0.0.2", 8080,
                ForwardAuth: ForwardAuthPolicy.Adaptive),
        };
        var options = new TraefikRenderOptions(
            HashiForwardAuthUrl: "http://127.0.0.1:18080/api/edge-auth/forward",
            HashiHealthUrl: "http://127.0.0.1:18080/api/health");

        var result = TraefikConfigRenderer.Render(resources, options);
        var generated = string.Concat(
            result.DynamicFiles.CoreYaml,
            result.DynamicFiles.HttpResourcesYaml,
            result.DynamicFiles.HealthYaml);

        Assert.Contains("http://127.0.0.1:18080/api/edge-auth/forward", result.DynamicFiles.CoreYaml);
        Assert.Contains("http://127.0.0.1:18080/api/health", result.DynamicFiles.HealthYaml);
        Assert.DoesNotContain("127.0.0.1:8080", generated, StringComparison.Ordinal);
    }

    [Fact]
    public async Task Platform_render_derives_hashi_internal_urls_from_configured_admin_port()
    {
        await using var db = CreateDb();
        db.Resources.Add(new ResourceEntity
        {
            Name = "App",
            Slug = "app",
            Kind = "http",
            Enabled = true,
            DashboardEnabled = true,
            Domain = "app.example.com",
            TargetScheme = "http",
            TargetHost = "10.0.0.2",
            TargetPort = 8080,
            ForwardAuthPolicy = "adaptive",
        });
        await db.SaveChangesAsync();

        var render = await TestPlatformHelpers
            .CreateTraefikPlatform(db, ports: new HashiPortOptions { Admin = 18080 })
            .RenderAsync();
        var generated = string.Concat(
            render.DynamicFiles.CoreYaml,
            render.DynamicFiles.HttpResourcesYaml,
            render.DynamicFiles.HealthYaml);

        Assert.Contains("http://127.0.0.1:18080/api/edge-auth/forward", render.DynamicFiles.CoreYaml);
        Assert.Contains("http://127.0.0.1:18080/api/health", render.DynamicFiles.HealthYaml);
        Assert.DoesNotContain("127.0.0.1:8080", generated, StringComparison.Ordinal);
    }

    [Fact]
    public async Task Platform_render_prefers_app_settings_internal_url()
    {
        await using var db = CreateDb();
        db.AppSettings.Add(new AppSettingsEntity
        {
            InternalUrl = "http://hashi.internal:19090/",
        });
        await db.SaveChangesAsync();

        var render = await TestPlatformHelpers
            .CreateTraefikPlatform(db, ports: new HashiPortOptions { Admin = 18080 })
            .RenderAsync();
        var generated = string.Concat(render.DynamicFiles.CoreYaml, render.DynamicFiles.HealthYaml);

        Assert.Contains("http://hashi.internal:19090/api/edge-auth/forward", render.DynamicFiles.CoreYaml);
        Assert.Contains("http://hashi.internal:19090/api/health", render.DynamicFiles.HealthYaml);
        Assert.DoesNotContain("127.0.0.1:8080", generated, StringComparison.Ordinal);
    }

    [Fact]
    public async Task Platform_render_includes_persisted_waf_exclusions()
    {
        await using var db = CreateDb();
        db.Resources.Add(new ResourceEntity
        {
            Name = "App",
            Slug = "app",
            Kind = "https",
            Enabled = true,
            Domain = "app.example.com",
            TargetScheme = "http",
            TargetHost = "10.0.0.2",
            TargetPort = 8080,
            WafMode = "on",
            WafExclusionsJson = JsonSerializer.Serialize(new[] { "SecRuleRemoveById 941100" }),
        });
        await db.SaveChangesAsync();

        var render = await TestPlatformHelpers.CreateTraefikPlatform(db).RenderAsync();

        Assert.Contains("app-waf:", render.DynamicFiles.SecurityYaml);
        Assert.Contains("SecRuleRemoveById 941100", render.DynamicFiles.SecurityYaml);
    }

    [Fact]
    public void Render_regex_rewrite_includes_replacement()
    {
        var resources = new List<ResourceDefinition>
        {
            new(Guid.NewGuid(), "App", "app", ResourceKind.Http, true, false, "app.example.com", "http", "10.0.0.2", 8080,
                Routes:
                [
                    new ResourceRouteDefinition(
                        100,
                        "regex",
                        "/old/(.*)",
                        "http",
                        "10.0.0.2",
                        8080,
                        RewriteMode: "regex",
                        RewriteValue: "^/old/(.*) => /new/$1"),
                ]),
        };

        var result = TraefikConfigRenderer.Render(resources);

        Assert.Contains("replacePathRegex:", result.DynamicFiles.HttpResourcesYaml);
        Assert.Contains("regex: \"^/old/(.*)\"", result.DynamicFiles.HttpResourcesYaml);
        Assert.Contains("replacement: \"/new/$1\"", result.DynamicFiles.HttpResourcesYaml);
        Assert.True(TraefikConfigValidator.ValidateRender(result).IsValid);
    }

    [Fact]
    public void Render_replace_prefix_rewrite_preserves_suffix()
    {
        var resources = new List<ResourceDefinition>
        {
            new(Guid.NewGuid(), "App", "app", ResourceKind.Http, true, false, "app.example.com", "http", "10.0.0.2", 8080,
                PathPrefix: "/api",
                PathRewriteMode: "replace_prefix",
                PathRewrite: "/v1"),
        };

        var result = TraefikConfigRenderer.Render(resources);

        Assert.Contains("replacePathRegex:", result.DynamicFiles.HttpResourcesYaml);
        Assert.Contains("regex: \"^/api(.*)\"", result.DynamicFiles.HttpResourcesYaml);
        Assert.Contains("replacement: \"/v1$1\"", result.DynamicFiles.HttpResourcesYaml);
        Assert.True(TraefikConfigValidator.ValidateRender(result).IsValid);
    }

    [Fact]
    public void Render_supports_replace_path_and_strip_prefix_rewrites()
    {
        var resources = new List<ResourceDefinition>
        {
            new(Guid.NewGuid(), "Exact", "exact", ResourceKind.Http, true, false, "exact.example.com", "http", "10.0.0.2", 8080,
                PathRewriteMode: "replace_path",
                PathRewrite: "/"),
            new(Guid.NewGuid(), "Strip", "strip", ResourceKind.Http, true, false, "strip.example.com", "http", "10.0.0.3", 8080,
                PathPrefix: "/api",
                PathRewriteMode: "strip_prefix",
                PathRewrite: "/api"),
        };

        var result = TraefikConfigRenderer.Render(resources);

        Assert.Contains("replacePath:", result.DynamicFiles.HttpResourcesYaml);
        Assert.Contains("path: \"/\"", result.DynamicFiles.HttpResourcesYaml);
        Assert.Contains("stripPrefix:", result.DynamicFiles.HttpResourcesYaml);
        Assert.Contains("- \"/api\"", result.DynamicFiles.HttpResourcesYaml);
        Assert.True(TraefikConfigValidator.ValidateRender(result).IsValid);
    }

    [Fact]
    public void Render_http_resource_without_domain_does_not_use_catch_all_host()
    {
        var resources = new List<ResourceDefinition>
        {
            new(Guid.NewGuid(), "Blank", "blank", ResourceKind.Http, true, false, null, "http", "10.0.0.2", 8080),
        };

        var result = TraefikConfigRenderer.Render(resources);

        Assert.DoesNotContain("HostRegexp", result.DynamicFiles.HttpResourcesYaml);
        Assert.DoesNotContain("blank:", result.DynamicFiles.HttpResourcesYaml);
        Assert.True(TraefikConfigValidator.ValidateRender(result).IsValid);
    }

    private static HashiDbContext CreateDb()
    {
        var options = new Microsoft.EntityFrameworkCore.DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        return new HashiDbContext(options);
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
        Assert.True(response.NetBirdEnabled);
        Assert.Equal("wt0", response.NetBirdInterface);
        Assert.True(response.NetBirdDetected);
        Assert.Equal(appliedAt, response.LastAppliedAtUtc);
    }
}

public sealed class TraefikStreamRendererTests
{
    [Fact]
    public void Render_tcp_resource_includes_proxy_protocol_when_enabled()
    {
        var resources = new List<ResourceDefinition>
        {
            new(
                Guid.NewGuid(),
                "Postgres",
                "postgres",
                ResourceKind.Tcp,
                true,
                false,
                null,
                "tcp",
                "10.0.0.5",
                5432,
                PublicPort: 15432,
                TcpProxyProtocolEnabled: true),
        };
        var options = new TraefikRenderOptions(ConfirmedStreamPorts: new HashSet<(int, string)> { (15432, "tcp") });

        var result = TraefikConfigRenderer.Render(resources, options);

        Assert.Contains("postgres-tcp:", result.StaticConfigYaml);
        Assert.Contains("proxyProtocol:", result.DynamicFiles.StreamResourcesYaml);
        Assert.Contains("version: 2", result.DynamicFiles.StreamResourcesYaml);
        Assert.True(TraefikConfigValidator.ValidateRender(result).IsValid);
    }

    [Fact]
    public void Render_includes_udp_entrypoint_when_port_confirmed()
    {
        var resources = new List<ResourceDefinition>
        {
            new(Guid.NewGuid(), "Game", "game", ResourceKind.Udp, true, false, null, "udp", "10.0.0.5", 27015, PublicPort: 27015),
        };
        var options = new TraefikRenderOptions(ConfirmedStreamPorts: new HashSet<(int, string)> { (27015, "udp") });
        var result = TraefikConfigRenderer.Render(resources, options);
        Assert.Contains("game-udp:", result.StaticConfigYaml);
        Assert.Contains(":27015/udp", result.StaticConfigYaml);
        Assert.Contains("udp:", result.DynamicFiles.StreamResourcesYaml);
    }

    [Theory]
    [InlineData(ResourceKind.Tcp, "tcp-only", "tcp")]
    [InlineData(ResourceKind.Udp, "udp-only", "udp")]
    public void Render_stream_resource_with_single_protocol_parses(ResourceKind kind, string slug, string protocol)
    {
        var resources = new List<ResourceDefinition>
        {
            new(Guid.NewGuid(), "Stream", slug, kind, true, false, null, protocol, "10.0.0.5", 25565, PublicPort: 25565),
        };
        var options = new TraefikRenderOptions(ConfirmedStreamPorts: new HashSet<(int, string)> { (25565, protocol) });
        var result = TraefikConfigRenderer.Render(resources, options);

        var validation = TraefikConfigValidator.ValidateRender(result);

        Assert.True(validation.IsValid, string.Join("; ", validation.Errors));
    }
}
