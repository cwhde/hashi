using System.Text.Json;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Platform;
using Hashi.Infrastructure.Services;
using Microsoft.EntityFrameworkCore;
using Xunit;

namespace Hashi.UnitTests;

public sealed class PublicDashboardServiceTests
{
    [Fact]
    public async Task GetAsync_returns_safe_resource_and_manual_dns_items_with_summary_counts()
    {
        await using var db = CreateDb();
        var resourceId = Guid.NewGuid();
        db.Resources.Add(new ResourceEntity
        {
            Id = resourceId,
            Name = "Resource App",
            Slug = "resource-app",
            Kind = "https",
            Enabled = true,
            Domain = "app.example.com",
            TargetScheme = "http",
            TargetHost = "10.0.0.10",
            TargetPort = 8080,
            DashboardEnabled = true,
            FirewallHostId = Guid.NewGuid(),
            PulseAgentId = Guid.NewGuid(),
            ForwardAuthPolicy = "sso_required",
            WafMode = "on",
            ExtraMiddlewaresJson = """["secret-chain"]""",
        });
        db.ResourceRoutes.Add(new ResourceRouteEntity
        {
            ResourceId = resourceId,
            TargetHost = "10.0.0.11",
            TargetPort = 9000,
            ExtraMiddlewaresJson = """["route-secret"]""",
        });
        db.ResourceRules.Add(new ResourceRuleEntity
        {
            ResourceId = resourceId,
            MatchValue = "/admin",
        });
        db.MonitorEndpoints.Add(new MonitorEndpointEntity
        {
            Name = "Resource App",
            ResourceId = resourceId,
            Enabled = true,
            Status = "up",
            LastLatencyMs = 25,
        });
        db.DnsRecords.Add(new DnsRecordEntity
        {
            ZoneId = Guid.NewGuid(),
            Name = "manual.example.com",
            Type = "A",
            Value = "203.0.113.20",
            Ownership = DnsOwnershipNames.User,
            Enabled = true,
            DashboardEnabled = true,
            DashboardDisplayName = "Manual DNS",
        });
        db.FirewallHosts.Add(new FirewallHostEntity
        {
            ConnectionId = Guid.NewGuid(),
            Name = "edge-1",
            Domain = "edge.example.com",
            LastAppliedAtUtc = DateTimeOffset.UtcNow,
        });
        await db.SaveChangesAsync();

        var dashboard = await CreateService(db).GetAsync();

        Assert.Equal(2, dashboard.TotalHosts);
        Assert.Equal(2, dashboard.HostsOnline);
        Assert.Equal(1, dashboard.TotalLinuxFirewallHosts);
        Assert.Equal(1, dashboard.LinuxFirewallHostsAvailable);
        Assert.Contains(dashboard.Items, x =>
            x.Source == "resource"
            && x.DisplayName == "Resource App"
            && x.PublicUrl == "https://app.example.com"
            && x.LastLatencyMs == 25);
        Assert.Contains(dashboard.Items, x =>
            x.Source == "manual_dns"
            && x.DisplayName == "Manual DNS"
            && x.PublicUrl == "https://manual.example.com");

        var json = JsonSerializer.Serialize(dashboard, new JsonSerializerOptions { PropertyNamingPolicy = JsonNamingPolicy.CamelCase });
        Assert.DoesNotContain("targetHost", json);
        Assert.DoesNotContain("targetPort", json);
        Assert.DoesNotContain("firewallHostId", json);
        Assert.DoesNotContain("pulseAgentId", json);
        Assert.DoesNotContain("routes", json);
        Assert.DoesNotContain("rules", json);
        Assert.DoesNotContain("forwardAuthPolicy", json);
        Assert.DoesNotContain("wafMode", json);
        Assert.DoesNotContain("middlewares", json, StringComparison.OrdinalIgnoreCase);
    }

    [Fact]
    public async Task GetAsync_omits_resource_without_public_url_instead_of_exposing_internal_target()
    {
        await using var db = CreateDb();
        db.Resources.Add(new ResourceEntity
        {
            Name = "Internal Only",
            Slug = "internal-only",
            Kind = "https",
            Enabled = true,
            TargetHost = "10.0.0.10",
            TargetPort = 8443,
            DashboardEnabled = true,
        });
        await db.SaveChangesAsync();

        var dashboard = await CreateService(db).GetAsync();

        Assert.Empty(dashboard.Items);
    }

    private static PublicDashboardService CreateService(HashiDbContext db)
        => new(db, new AppSettingsService(db));

    private static HashiDbContext CreateDb()
    {
        var options = new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        return new HashiDbContext(options);
    }
}
