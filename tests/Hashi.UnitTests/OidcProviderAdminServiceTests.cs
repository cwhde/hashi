using Hashi.Contracts.Api;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Platform;
using Hashi.Infrastructure.Services;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Configuration;
using Microsoft.Extensions.Logging.Abstractions;
using Xunit;

namespace Hashi.UnitTests;

public sealed class OidcProviderAdminServiceTests
{
    [Fact]
    public async Task CreateRuleAsync_rejects_enabled_geo_rule_when_geoip_database_is_unavailable()
    {
        await using var db = CreateDb();
        var service = CreateService(db);

        var ex = await Assert.ThrowsAsync<InvalidOperationException>(() =>
            service.CreateRuleAsync(new CreateEdgeAuthRuleRequest(
                "US only",
                10,
                """{"country":"US"}""",
                "allow",
                Enabled: true)));

        Assert.Contains("GeoIP database", ex.Message, StringComparison.Ordinal);
    }

    [Fact]
    public async Task CreateRuleAsync_allows_disabled_geo_rule_when_geoip_database_is_unavailable()
    {
        await using var db = CreateDb();
        var service = CreateService(db);

        var rule = await service.CreateRuleAsync(new CreateEdgeAuthRuleRequest(
            "US only",
            10,
            """{"country":"US"}""",
            "allow",
            Enabled: false));

        Assert.False(rule.Enabled);
    }

    private static HashiDbContext CreateDb()
    {
        var options = new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        return new HashiDbContext(options);
    }

    private static OidcProviderAdminService CreateService(HashiDbContext db)
    {
        var config = new ConfigurationBuilder()
            .AddInMemoryCollection(new Dictionary<string, string?>
            {
                ["Hashi:DataPath"] = Path.Combine(Path.GetTempPath(), Guid.NewGuid().ToString()),
            })
            .Build();
        return new OidcProviderAdminService(
            db,
            new SecretRecordService(db, new VaultSessionState(), new ServiceSyncVaultState()),
            new AuditService(db),
            new GeoIpLookupService(config, NullLogger<GeoIpLookupService>.Instance));
    }
}
