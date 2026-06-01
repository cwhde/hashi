using Hashi.Contracts.Api;
using Hashi.Core.Hosting;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Platform;
using Hashi.Infrastructure.Services;
using Microsoft.EntityFrameworkCore;
using Xunit;

namespace Hashi.UnitTests;

public sealed class MonitoringServiceTests
{
    [Fact]
    public async Task SyncEndpointsFromResourcesAsync_maps_resource_kinds_to_supported_check_types()
    {
        await using var db = CreateDb();
        db.Resources.AddRange(
            Resource("web", "http", "app.example.com", "10.0.0.10", 8080),
            Resource("secure", "https", "secure.example.com", "10.0.0.11", 8443),
            Resource("grpc", "h2c", "grpc.example.com", "10.0.0.12", 5000),
            Resource("postgres", "tcp", null, "10.0.0.13", 5432),
            Resource("wireguard", "udp", null, "10.0.0.14", 51820),
            Resource("dynamic", "pulse", null, "10.0.0.15", 80));
        await db.SaveChangesAsync();

        await CreateService(db).SyncEndpointsFromResourcesAsync();

        var endpoints = await db.MonitorEndpoints
            .Where(x => x.ResourceId != null)
            .OrderBy(x => x.Name)
            .ToListAsync();
        Assert.Collection(
            endpoints,
            x =>
            {
                Assert.Equal("dynamic", x.Name);
                Assert.Equal("pulse", x.CheckType);
                Assert.StartsWith("pulse://", x.Url, StringComparison.Ordinal);
            },
            x =>
            {
                Assert.Equal("grpc", x.Name);
                Assert.Equal("h2c", x.CheckType);
                Assert.Equal("http://grpc.example.com/", x.Url);
            },
            x =>
            {
                Assert.Equal("postgres", x.Name);
                Assert.Equal("tcp", x.CheckType);
                Assert.Equal("tcp://10.0.0.13:5432", x.Url);
            },
            x =>
            {
                Assert.Equal("secure", x.Name);
                Assert.Equal("https", x.CheckType);
            },
            x =>
            {
                Assert.Equal("web", x.Name);
                Assert.Equal("http", x.CheckType);
            },
            x =>
            {
                Assert.Equal("wireguard", x.Name);
                Assert.Equal("udp", x.CheckType);
                Assert.Equal("udp://10.0.0.14:51820", x.Url);
            });
    }

    [Fact]
    public async Task SyncEndpointsFromResourcesAsync_provisions_required_infrastructure_sources()
    {
        await using var db = CreateDb();
        db.Connections.Add(new ConnectionEntity
        {
            Name = "edge-1",
            Type = ConnectionTypeNames.TraefikHost,
            Enabled = true,
            SettingsJson = """{"Host":"10.0.0.20","Port":22}""",
        });
        db.FirewallHosts.Add(new FirewallHostEntity
        {
            Name = "fw-1",
            Domain = "fw.example.com",
            LinkedTraefikHost = "traefik.internal",
            InternalTraefikIp = "10.0.0.2",
        });
        db.AdGuardConnections.Add(new AdGuardConnectionEntity
        {
            Name = "dns-filter",
            BaseUrl = "http://10.0.0.53:3000",
            Enabled = true,
        });
        db.DnsZones.Add(new DnsZoneEntity
        {
            Id = Guid.Parse("11111111-1111-1111-1111-111111111111"),
            ConnectionId = Guid.NewGuid(),
            ProviderZoneId = "zone",
            Name = "example.com",
        });
        db.DnsRecords.Add(new DnsRecordEntity
        {
            ZoneId = Guid.Parse("11111111-1111-1111-1111-111111111111"),
            Name = "manual.example.com",
            Type = "A",
            Value = "192.0.2.10",
            Ownership = DnsOwnershipNames.User,
            Enabled = true,
        });
        db.PulseAgents.Add(new PulseAgentEntity
        {
            Name = "laptop",
            LastPublicIp = "198.51.100.8",
            Status = "healthy",
        });
        await db.SaveChangesAsync();

        await CreateService(db).SyncEndpointsFromResourcesAsync();

        var endpoints = await db.MonitorEndpoints.ToDictionaryAsync(x => x.Name, x => x);
        Assert.Equal("http", endpoints["Hashi API"].CheckType);
        Assert.Equal("icmp", endpoints["Firewall: fw-1"].CheckType);
        Assert.Equal("tcp", endpoints["Traefik SSH: edge-1"].CheckType);
        Assert.Equal("http", endpoints["AdGuard: dns-filter"].CheckType);
        Assert.Equal("dns", endpoints["DNS: manual.example.com"].CheckType);
        Assert.Equal("icmp", endpoints["Pulse network: laptop"].CheckType);
    }

    [Fact]
    public async Task SyncEndpointsFromResourcesAsync_uses_configured_admin_port_for_hashi_api_monitor()
    {
        await using var db = CreateDb();

        await CreateService(db, new HashiPortOptions { Admin = 18080 }).SyncEndpointsFromResourcesAsync();

        var endpoint = await db.MonitorEndpoints.SingleAsync(x => x.Name == "Hashi API");
        Assert.Equal("http://127.0.0.1:18080/api/health", endpoint.Url);
        Assert.DoesNotContain("127.0.0.1:8080", endpoint.Url, StringComparison.Ordinal);
    }

    [Fact]
    public async Task SyncEndpointsFromResourcesAsync_prefers_internal_url_for_hashi_api_monitor()
    {
        await using var db = CreateDb();
        db.AppSettings.Add(new AppSettingsEntity
        {
            InternalUrl = "http://hashi.internal:19090/",
        });
        await db.SaveChangesAsync();

        await CreateService(db, new HashiPortOptions { Admin = 18080 }).SyncEndpointsFromResourcesAsync();

        var endpoint = await db.MonitorEndpoints.SingleAsync(x => x.Name == "Hashi API");
        Assert.Equal("http://hashi.internal:19090/api/health", endpoint.Url);
        Assert.DoesNotContain("127.0.0.1:8080", endpoint.Url, StringComparison.Ordinal);
    }

    [Fact]
    public async Task Manual_endpoint_crud_validates_check_type_and_preserves_resource_owned_rows()
    {
        await using var db = CreateDb();
        var service = CreateService(db);
        var created = await service.CreateManualAsync(new CreateMonitorEndpointRequest(
            "Manual TCP",
            "tcp://db.internal:5432",
            "tcp"));

        Assert.Equal("tcp", created.CheckType);

        var updated = await service.UpdateManualAsync(
            created.Id,
            new UpdateMonitorEndpointRequest(Url: "udp://dns.internal:53", CheckType: "udp", Enabled: false));
        Assert.NotNull(updated);
        Assert.Equal("udp", updated.CheckType);
        Assert.False(updated.Enabled);

        await Assert.ThrowsAsync<InvalidOperationException>(() =>
            service.CreateManualAsync(new CreateMonitorEndpointRequest("Bad", "bad://thing", "smtp")));

        db.MonitorEndpoints.Add(new MonitorEndpointEntity
        {
            Name = "Owned",
            Url = "http://owned.example.com/",
            CheckType = "http",
            ResourceId = Guid.NewGuid(),
        });
        await db.SaveChangesAsync();
        var owned = await db.MonitorEndpoints.SingleAsync(x => x.Name == "Owned");
        await Assert.ThrowsAsync<InvalidOperationException>(() =>
            service.DeleteManualAsync(owned.Id));

        Assert.True(await service.DeleteManualAsync(created.Id));
        Assert.False(await db.MonitorEndpoints.AnyAsync(x => x.Id == created.Id));
    }

    private static ResourceEntity Resource(string name, string kind, string? domain, string host, int port)
        => new()
        {
            Name = name,
            Slug = name,
            Kind = kind,
            Domain = domain,
            TargetScheme = kind == "https" ? "https" : "http",
            TargetHost = host,
            TargetPort = port,
            StatusEnabled = true,
            Enabled = true,
        };

    private static MonitoringService CreateService(HashiDbContext db, HashiPortOptions? ports = null)
        => new(db, new AppSettingsService(db), new HashiInternalUrlResolver(ports ?? new HashiPortOptions()));

    private static HashiDbContext CreateDb()
    {
        var options = new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        return new HashiDbContext(options);
    }
}
