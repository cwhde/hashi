using Hashi.Contracts.Api;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Platform;
using Hashi.UnitTests.Fakes;
using Microsoft.EntityFrameworkCore;
using Xunit;

namespace Hashi.UnitTests;

public sealed class ResourceServiceHintTests
{
    [Fact]
    public async Task Create_update_response_and_definitions_include_resource_hints()
    {
        await using var db = CreateDb();
        var service = TestPlatformHelpers.CreateResourceService(db);

        var created = await service.CreateAsync(new CreateResourceRequest(
            "Postgres",
            "tcp",
            "db.example.com",
            "tcp",
            "10.0.0.5",
            5432,
            DashboardEnabled: false,
            StatusEnabled: true,
            PublicPort: 15432,
            TcpProxyProtocolEnabled: true,
            MonitoringProtocolHint: "TLS",
            DomainMode: "custom"));

        var response = await service.ToResponseAsync(created);
        var stored = await db.Resources.AsNoTracking().SingleAsync(x => x.Id == created.Id);
        var definition = Assert.Single(await ResourceService.BuildDefinitionsAsync(db));

        Assert.True(stored.TcpProxyProtocolEnabled);
        Assert.Equal("tls", stored.MonitoringProtocolHint);
        Assert.True(response.TcpProxyProtocolEnabled);
        Assert.Equal("tls", response.MonitoringProtocolHint);
        Assert.True(definition.TcpProxyProtocolEnabled);
        Assert.Equal("tls", definition.MonitoringProtocolHint);

        await service.UpdateAsync(
            created.Id,
            new UpdateResourceRequest(
                null,
                null,
                null,
                null,
                null,
                null,
                null,
                null,
                TcpProxyProtocolEnabled: false,
                ClearMonitoringProtocolHint: true));

        var updated = await db.Resources.AsNoTracking().SingleAsync(x => x.Id == created.Id);
        Assert.False(updated.TcpProxyProtocolEnabled);
        Assert.Null(updated.MonitoringProtocolHint);
    }

    [Fact]
    public async Task CreateAsync_rejects_tcp_proxy_protocol_for_non_tcp_resources()
    {
        await using var db = CreateDb();
        var service = TestPlatformHelpers.CreateResourceService(db);

        var ex = await Assert.ThrowsAsync<InvalidOperationException>(() => service.CreateAsync(
            new CreateResourceRequest(
                "Web",
                "https",
                "web.example.com",
                "http",
                "10.0.0.5",
                8080,
                DashboardEnabled: true,
                StatusEnabled: true,
                TcpProxyProtocolEnabled: true,
                DomainMode: "custom")));

        Assert.Contains("TCP proxy protocol", ex.Message);
    }

    private static HashiDbContext CreateDb()
    {
        var options = new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        return new HashiDbContext(options);
    }
}
