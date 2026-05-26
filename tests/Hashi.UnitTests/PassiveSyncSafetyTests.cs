using Hashi.Core.Auth;
using Hashi.Core.Dns;
using Hashi.Core.Sync;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Crypto;
using Hashi.Infrastructure.Dns;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Platform;
using Hashi.Infrastructure.Services;
using Hashi.Infrastructure.Sync;
using Hashi.UnitTests.Fakes;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Xunit;

namespace Hashi.UnitTests;

public sealed class PassiveSyncSafetyTests
{
    [Fact]
    public async Task ReconcileAsync_persists_awaiting_confirmation_when_dns_deletes_pending()
    {
        var options = new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        await using var db = new HashiDbContext(options);
        db.AppSettings.Add(new AppSettingsEntity { RootDomain = "example.com" });

        var secretId = Guid.NewGuid();
        var rootKey = new byte[32];
        var dek = new byte[32];
        var token = "dns-token";
        db.SecretRecords.Add(new SecretRecordEntity
        {
            Id = secretId,
            Purpose = SecretPurposeMapping.ToName(SecretPurpose.DnsProviderToken),
            Label = "DNS token",
            AdminWrappedDekBlob = AesGcmCipher.Encrypt(dek, rootKey).ToBlob(),
            CiphertextBlob = AesGcmCipher.Encrypt(System.Text.Encoding.UTF8.GetBytes(token), dek).ToBlob(),
        });

        var connectionId = Guid.NewGuid();
        db.Connections.Add(new ConnectionEntity
        {
            Id = connectionId,
            Name = "hetzner",
            Type = ConnectionTypeNames.DnsProvider,
            Enabled = true,
            SecretId = secretId,
            SettingsJson = """{"provider":"hetzner","zoneName":"example.com","defaultTtl":3600}""",
        });
        db.DnsZones.Add(new DnsZoneEntity
        {
            ConnectionId = connectionId,
            ProviderZoneId = "zone-1",
            Name = "example.com",
            DefaultTtl = 3600,
        });
        await db.SaveChangesAsync();

        var providerFactory = new TestDnsProviderFactory();
        providerFactory.Provider.SeedZone(
            "zone-1",
            "example.com",
            new DnsRecordSnapshot("stale", "stale.example.com", DnsRecordType.A, "203.0.113.99", 3600, true),
            new DnsRecordSnapshot("keep", "app.example.com", DnsRecordType.A, "203.0.113.10", 3600, false));

        var vault = new VaultSessionState();
        vault.Unlock(rootKey);
        var secrets = new SecretRecordService(db, vault, new ServiceSyncVaultState());
        var dns = new DnsConnectionService(db, providerFactory, secrets, new AuditService(db));
        var settings = new AppSettingsService(db);
        var userMiddlewares = new TraefikUserMiddlewareService(db);
        var syncRuns = new SyncRunService(db);
        var orchestrator = new SyncOrchestratorService(
            db,
            dns,
            new TraefikPlatformService(db, settings, userMiddlewares),
            new TraefikSyncService(db, new FakeSshRemoteExecutor(), new TraefikPlatformService(db, settings, userMiddlewares), secrets, new AuditService(db)),
            new FirewallApplyService(db, new FakeSshRemoteExecutor(), secrets, new AuditService(db)),
            new AdGuardSyncService(db, new ServiceCollection().AddHttpClient().BuildServiceProvider().GetRequiredService<IHttpClientFactory>(), secrets),
            syncRuns,
            settings,
            new AuditService(db));

        var result = await orchestrator.ReconcileAsync();

        Assert.True(result.Succeeded);
        var run = await db.SyncRuns.Include(x => x.Diffs).SingleAsync(x => x.Id == result.RunId);
        Assert.Equal(SyncRunStatusNames.AwaitingConfirmation, run.Status);
        Assert.Contains(run.Diffs, d => d.ChangeKind == nameof(ProviderResultKind.Deleted));
        var remaining = await providerFactory.Provider.ListRecordsAsync("zone-1");
        Assert.Contains(remaining, r => r.Name == "stale.example.com");
    }

    [Fact]
    public async Task ApplySafePlanAsync_skips_deletes_but_applies_creates()
    {
        var options = new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        await using var db = new HashiDbContext(options);

        var secretId = Guid.NewGuid();
        var rootKey = new byte[32];
        var dek = new byte[32];
        db.SecretRecords.Add(new SecretRecordEntity
        {
            Id = secretId,
            Purpose = SecretPurposeMapping.ToName(SecretPurpose.DnsProviderToken),
            Label = "DNS token",
            AdminWrappedDekBlob = AesGcmCipher.Encrypt(dek, rootKey).ToBlob(),
            CiphertextBlob = AesGcmCipher.Encrypt(System.Text.Encoding.UTF8.GetBytes("token"), dek).ToBlob(),
        });
        var connectionId = Guid.NewGuid();
        db.Connections.Add(new ConnectionEntity
        {
            Id = connectionId,
            Name = "hetzner",
            Type = ConnectionTypeNames.DnsProvider,
            Enabled = true,
            SecretId = secretId,
            SettingsJson = """{"provider":"hetzner","zoneName":"example.com","defaultTtl":3600}""",
        });
        db.DnsZones.Add(new DnsZoneEntity
        {
            ConnectionId = connectionId,
            ProviderZoneId = "zone-1",
            Name = "example.com",
            DefaultTtl = 3600,
        });
        await db.SaveChangesAsync();

        var providerFactory = new TestDnsProviderFactory();
        providerFactory.Provider.SeedZone(
            "zone-1",
            "example.com",
            new DnsRecordSnapshot("stale", "stale.example.com", DnsRecordType.A, "203.0.113.99", 3600, false));

        var vault = new VaultSessionState();
        vault.Unlock(rootKey);
        var secrets = new SecretRecordService(db, vault, new ServiceSyncVaultState());
        var dns = new DnsConnectionService(db, providerFactory, secrets, new AuditService(db));
        var plan = new DnsSyncPlan(
            Guid.NewGuid(),
            connectionId,
            "example.com",
            [
                new DnsPlanChange(DnsChangeKind.Delete, "stale.example.com", DnsRecordType.A, "203.0.113.99", null, 3600, "Remove stale record"),
                new DnsPlanChange(DnsChangeKind.Create, "new.example.com", DnsRecordType.A, null, "203.0.113.20", 3600, "Add record"),
            ],
            RequiresConfirmation: true);

        await dns.ApplySafePlanAsync(plan);

        var records = await providerFactory.Provider.ListRecordsAsync("zone-1");
        Assert.Contains(records, r => r.Name == "stale.example.com");
        Assert.Contains(records, r => r.Name == "new.example.com");
    }
}
