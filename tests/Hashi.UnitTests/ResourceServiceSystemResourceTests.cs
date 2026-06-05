using Hashi.Contracts.Api;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.UnitTests.Fakes;
using Microsoft.EntityFrameworkCore;
using Xunit;

namespace Hashi.UnitTests;

public sealed class ResourceServiceSystemResourceTests
{
    [Fact]
    public async Task UpdateAsync_rejects_system_resources_before_mutating_fields()
    {
        await using var db = CreateDb();
        var resource = new ResourceEntity
        {
            Name = "System",
            Slug = "system",
            Kind = "https",
            Enabled = true,
            IsSystem = true,
            Domain = "system.example.com",
            TargetScheme = "http",
            TargetHost = "127.0.0.1",
            TargetPort = 8080,
        };
        db.Resources.Add(resource);
        await db.SaveChangesAsync();
        var service = TestPlatformHelpers.CreateResourceService(db);

        var ex = await Assert.ThrowsAsync<InvalidOperationException>(() => service.UpdateAsync(
            resource.Id,
            new UpdateResourceRequest(
                "Renamed",
                false,
                "renamed.example.com",
                "https",
                "10.0.0.10",
                8443,
                false,
                false)));

        Assert.Contains("System resources cannot be updated", ex.Message);
        var stored = await db.Resources.AsNoTracking().SingleAsync(x => x.Id == resource.Id);
        Assert.Equal("System", stored.Name);
        Assert.True(stored.Enabled);
        Assert.Equal("system.example.com", stored.Domain);
        Assert.Equal("http", stored.TargetScheme);
        Assert.Equal("127.0.0.1", stored.TargetHost);
        Assert.Equal(8080, stored.TargetPort);
    }

    [Fact]
    public async Task CreateAsync_rejects_internal_agent_dns_domain_for_reverse_proxy_resources()
    {
        await using var db = CreateDb();
        db.InternalAgentDnsSettings.Add(new InternalAgentDnsSettingsEntity
        {
            Enabled = true,
            Domain = "hashi.home.arpa",
        });
        await db.SaveChangesAsync();
        var service = TestPlatformHelpers.CreateResourceService(db);

        var ex = await Assert.ThrowsAsync<InvalidOperationException>(() =>
            service.CreateAsync(new CreateResourceRequest(
                "Edge",
                "https",
                "edge.hashi.home.arpa",
                "http",
                "10.0.0.42",
                8080,
                false,
                false,
                DomainMode: "custom")));

        Assert.Contains("DNS-only", ex.Message);
        Assert.Empty(await db.Resources.ToListAsync());
    }

    private static HashiDbContext CreateDb()
    {
        var options = new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        return new HashiDbContext(options);
    }
}
