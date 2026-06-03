using Hashi.Contracts.Api;
using Hashi.Core.Resources;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.UnitTests.Fakes;
using Microsoft.EntityFrameworkCore;
using Xunit;

namespace Hashi.UnitTests;

public sealed class ResourceServiceRuleValidationTests
{
    [Fact]
    public async Task CreateAsync_rejects_enabled_geoip_rule_when_geoip_is_unavailable()
    {
        await using var db = CreateDb();
        var service = TestPlatformHelpers.CreateResourceService(db);

        var ex = await Assert.ThrowsAsync<InvalidOperationException>(() => service.CreateAsync(CreateRequest(
            [new ResourceRuleRequest(true, 100, ResourceRuleActionNames.BlockAccess, ResourceRuleMatchTypeNames.Country, "US")])));

        Assert.Contains("GeoIP database", ex.Message);
        Assert.Empty(await db.Resources.AsNoTracking().ToListAsync());
        Assert.Empty(await db.ResourceRules.AsNoTracking().ToListAsync());
    }

    [Fact]
    public async Task CreateAsync_allows_non_geoip_and_disabled_geoip_rules_without_geoip_database()
    {
        await using var db = CreateDb();
        var service = TestPlatformHelpers.CreateResourceService(db);

        var created = await service.CreateAsync(CreateRequest(
            [
                new ResourceRuleRequest(true, 100, "allow", "path", "/admin"),
                new ResourceRuleRequest(false, 90, "block", "asn", "AS13335"),
            ]));

        var storedRules = await db.ResourceRules.AsNoTracking().Where(x => x.ResourceId == created.Id).ToListAsync();
        Assert.Equal(2, storedRules.Count);
        Assert.Contains(storedRules, x => x.Enabled && x.Action == ResourceRuleActionNames.BypassAuth && x.MatchType == ResourceRuleMatchTypeNames.Path);
        Assert.Contains(storedRules, x => !x.Enabled && x.Action == ResourceRuleActionNames.BlockAccess && x.MatchType == ResourceRuleMatchTypeNames.Asn);
    }

    [Fact]
    public async Task CreateAsync_rejects_unknown_rule_action_before_storage()
    {
        await using var db = CreateDb();
        var service = TestPlatformHelpers.CreateResourceService(db);

        var ex = await Assert.ThrowsAsync<InvalidOperationException>(() => service.CreateAsync(CreateRequest(
            [new ResourceRuleRequest(true, 100, "permit", ResourceRuleMatchTypeNames.Path, "/admin")])));

        Assert.Contains("Resource rule action", ex.Message);
        Assert.Empty(await db.Resources.AsNoTracking().ToListAsync());
        Assert.Empty(await db.ResourceRules.AsNoTracking().ToListAsync());
    }

    [Fact]
    public async Task CreateAsync_rejects_unknown_rule_match_type_before_storage()
    {
        await using var db = CreateDb();
        var service = TestPlatformHelpers.CreateResourceService(db);

        var ex = await Assert.ThrowsAsync<InvalidOperationException>(() => service.CreateAsync(CreateRequest(
            [new ResourceRuleRequest(true, 100, ResourceRuleActionNames.BlockAccess, "header", "x-test")])));

        Assert.Contains("Resource rule match type", ex.Message);
        Assert.Empty(await db.Resources.AsNoTracking().ToListAsync());
        Assert.Empty(await db.ResourceRules.AsNoTracking().ToListAsync());
    }

    [Fact]
    public async Task UpdateAsync_rejects_enabled_geoip_rule_before_mutating_resource()
    {
        await using var db = CreateDb();
        var resource = new ResourceEntity
        {
            Name = "App",
            Slug = "app",
            Kind = "https",
            Enabled = true,
            Domain = "app.example.com",
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
                null,
                null,
                null,
                null,
                null,
                null,
                null,
                Rules: [new ResourceRuleRequest(true, 100, ResourceRuleActionNames.BlockAccess, ResourceRuleMatchTypeNames.Region, "ZH")])));

        Assert.Contains("GeoIP database", ex.Message);
        var stored = await db.Resources.AsNoTracking().SingleAsync(x => x.Id == resource.Id);
        Assert.Equal("App", stored.Name);
        Assert.Empty(await db.ResourceRules.AsNoTracking().ToListAsync());
    }

    private static CreateResourceRequest CreateRequest(IReadOnlyList<ResourceRuleRequest> rules)
        => new(
            "App",
            "https",
            "app.example.com",
            "http",
            "127.0.0.1",
            8080,
            true,
            true,
            Rules: rules);

    private static HashiDbContext CreateDb()
    {
        var options = new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        return new HashiDbContext(options);
    }
}
