using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.UnitTests.Fakes;
using Microsoft.EntityFrameworkCore;
using Xunit;

namespace Hashi.UnitTests;

public sealed class TraefikRenderOutputTests
{
    [Fact]
    public async Task RenderAsync_includes_http_resources()
    {
        await using var db = CreateDb();
        db.Resources.Add(new ResourceEntity
        {
            Name = "Web App",
            Slug = "web-app",
            Kind = "https",
            Enabled = true,
            Domain = "web.example.com",
            TargetScheme = "http",
            TargetHost = "10.0.0.10",
            TargetPort = 8080,
        });
        await db.SaveChangesAsync();

        var traefik = TestPlatformHelpers.CreateTraefikPlatform(db);
        var render = await traefik.RenderAsync();

        Assert.Contains("web.example.com", render.DynamicFiles.HttpResourcesYaml);
        Assert.Contains("10.0.0.10", render.DynamicFiles.HttpResourcesYaml);
    }

    [Fact]
    public async Task RenderAsync_includes_tcp_resources()
    {
        await using var db = CreateDb();
        db.Resources.Add(new ResourceEntity
        {
            Name = "TCP App",
            Slug = "tcp-app",
            Kind = "tcp",
            Enabled = true,
            TargetHost = "10.0.0.20",
            TargetPort = 5432,
            PublicPort = 5432,
        });
        db.TraefikEntryPoints.Add(new TraefikEntryPointEntity { Port = 5432, Protocol = "tcp", Confirmed = true });
        await db.SaveChangesAsync();

        var traefik = TestPlatformHelpers.CreateTraefikPlatform(db);
        var render = await traefik.RenderAsync();

        Assert.Contains("5432", render.DynamicFiles.StreamResourcesYaml);
    }

    [Fact]
    public async Task RenderAsync_includes_udp_resources()
    {
        await using var db = CreateDb();
        db.Resources.Add(new ResourceEntity
        {
            Name = "UDP App",
            Slug = "udp-app",
            Kind = "udp",
            Enabled = true,
            TargetHost = "10.0.0.30",
            TargetPort = 51820,
            PublicPort = 51820,
        });
        db.TraefikEntryPoints.Add(new TraefikEntryPointEntity { Port = 51820, Protocol = "udp", Confirmed = true });
        await db.SaveChangesAsync();

        var traefik = TestPlatformHelpers.CreateTraefikPlatform(db);
        var render = await traefik.RenderAsync();

        Assert.Contains("51820", render.DynamicFiles.StreamResourcesYaml);
    }

    [Fact]
    public async Task RenderAsync_skips_disabled_resources()
    {
        await using var db = CreateDb();
        db.Resources.Add(new ResourceEntity
        {
            Name = "Disabled App",
            Slug = "disabled-app",
            Kind = "https",
            Enabled = false,
            Domain = "disabled.example.com",
            TargetScheme = "http",
            TargetHost = "10.0.0.40",
            TargetPort = 8080,
        });
        await db.SaveChangesAsync();

        var traefik = TestPlatformHelpers.CreateTraefikPlatform(db);
        var render = await traefik.RenderAsync();

        Assert.DoesNotContain("disabled.example.com", render.DynamicFiles.HttpResourcesYaml);
    }

    [Fact]
    public async Task RenderAsync_content_hash_is_stable_for_same_input()
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
            TargetHost = "10.0.0.10",
            TargetPort = 8080,
        });
        await db.SaveChangesAsync();

        var traefik = TestPlatformHelpers.CreateTraefikPlatform(db);
        var render1 = await traefik.RenderAsync();
        var render2 = await traefik.RenderAsync();

        Assert.Equal(render1.ContentHash, render2.ContentHash);
    }

    [Fact]
    public async Task RenderAsync_includes_middlewares_in_http_resources()
    {
        await using var db = CreateDb();
        db.TraefikUserMiddlewares.Add(new TraefikUserMiddlewareEntity
        {
            Yaml = "http:\n  middlewares:\n    rate-limit:\n      rateLimit:\n        average: 100",
        });
        await db.SaveChangesAsync();

        var traefik = TestPlatformHelpers.CreateTraefikPlatform(db);
        var render = await traefik.RenderAsync();

        Assert.Contains("rate-limit", render.DynamicFiles.UserMiddlewaresYaml);
    }

    [Fact]
    public async Task RenderAsync_static_config_includes_entrypoints()
    {
        await using var db = CreateDb();
        db.TraefikEntryPoints.AddRange(
            new TraefikEntryPointEntity { Port = 80, Protocol = "tcp", Confirmed = true },
            new TraefikEntryPointEntity { Port = 443, Protocol = "tcp", Confirmed = true });
        await db.SaveChangesAsync();

        var traefik = TestPlatformHelpers.CreateTraefikPlatform(db);
        var render = await traefik.RenderAsync();

        Assert.Contains("80", render.StaticConfigYaml);
        Assert.Contains("443", render.StaticConfigYaml);
    }

    [Fact]
    public async Task RenderAsync_includes_health_endpoint()
    {
        await using var db = CreateDb();
        await db.SaveChangesAsync();

        var traefik = TestPlatformHelpers.CreateTraefikPlatform(db);
        var render = await traefik.RenderAsync();

        Assert.Contains("health", render.DynamicFiles.HealthYaml, StringComparison.OrdinalIgnoreCase);
    }

    private static HashiDbContext CreateDb()
    {
        var options = new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        return new HashiDbContext(options);
    }
}
