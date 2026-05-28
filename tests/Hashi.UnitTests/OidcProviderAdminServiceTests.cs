using Hashi.Contracts.Api;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Platform;
using Hashi.Infrastructure.Services;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Configuration;
using Microsoft.Extensions.Logging.Abstractions;
using System.Security.Cryptography;
using System.Text;
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

    [Fact]
    public async Task CreateProviderAsync_stores_client_secret_for_service_sync()
    {
        await using var db = CreateDb();
        var serviceSync = ReadyServiceSync();
        var secrets = CreateSecrets(db, serviceSync);
        var service = CreateService(db, secrets);

        var provider = await service.CreateProviderAsync(new CreateOidcProviderRequest(
            "Provider",
            "https://issuer.example",
            "client-id",
            "client-secret",
            Scopes: null,
            Enabled: true));

        var secret = await db.SecretRecords.SingleAsync(x => x.Id == db.OidcProviders.Single(y => y.Id == provider.Id).ClientSecretId);
        Assert.Equal(SecretPurposeNames.OidcClientSecret, secret.Purpose);
        Assert.True(secret.IsServiceSyncEligible);
        Assert.NotNull(secret.ServiceWrappedDekBlob);
        Assert.Equal("client-secret", Encoding.UTF8.GetString((await secrets.DecryptForServiceSyncAsync(secret.Id))!));
    }

    [Fact]
    public async Task UpdateProviderAsync_stores_replacement_client_secret_for_service_sync()
    {
        await using var db = CreateDb();
        var serviceSync = ReadyServiceSync();
        var secrets = CreateSecrets(db, serviceSync);
        var service = CreateService(db, secrets);
        var provider = await service.CreateProviderAsync(new CreateOidcProviderRequest(
            "Provider",
            "https://issuer.example",
            "client-id",
            "client-secret",
            Scopes: null,
            Enabled: true));

        await service.UpdateProviderAsync(provider.Id, new UpdateOidcProviderRequest(
            Name: null,
            Issuer: null,
            ClientId: null,
            ClientSecret: "replacement-secret",
            Scopes: null,
            Enabled: null));

        var entity = await db.OidcProviders.SingleAsync(x => x.Id == provider.Id);
        var secret = await db.SecretRecords.SingleAsync(x => x.Id == entity.ClientSecretId);
        Assert.True(secret.IsServiceSyncEligible);
        Assert.NotNull(secret.ServiceWrappedDekBlob);
        Assert.Equal("replacement-secret", Encoding.UTF8.GetString((await secrets.DecryptForServiceSyncAsync(secret.Id))!));
    }

    private static HashiDbContext CreateDb()
    {
        var options = new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        return new HashiDbContext(options);
    }

    private static OidcProviderAdminService CreateService(HashiDbContext db, SecretRecordService? secrets = null)
    {
        var config = new ConfigurationBuilder()
            .AddInMemoryCollection(new Dictionary<string, string?>
            {
                ["Hashi:DataPath"] = Path.Combine(Path.GetTempPath(), Guid.NewGuid().ToString()),
            })
            .Build();
        return new OidcProviderAdminService(
            db,
            secrets ?? new SecretRecordService(db, new VaultSessionState(), new ServiceSyncVaultState()),
            new AuditService(db),
            new GeoIpLookupService(config, NullLogger<GeoIpLookupService>.Instance));
    }

    private static SecretRecordService CreateSecrets(HashiDbContext db, ServiceSyncVaultState serviceSync)
    {
        var vault = new VaultSessionState();
        vault.Unlock(RandomNumberGenerator.GetBytes(32));
        return new SecretRecordService(db, vault, serviceSync);
    }

    private static ServiceSyncVaultState ReadyServiceSync()
    {
        var serviceSync = new ServiceSyncVaultState();
        serviceSync.Initialize(RandomNumberGenerator.GetBytes(32));
        return serviceSync;
    }
}
