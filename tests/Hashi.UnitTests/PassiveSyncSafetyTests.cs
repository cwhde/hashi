using System.Net;
using System.Text;
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
        var syncRuns = new SyncRunService(db);
        var orchestrator = new SyncOrchestratorService(
            db,
            dns,
            TestPlatformHelpers.CreateTraefikPlatform(db, vault),
            TestPlatformHelpers.CreateTraefikSync(db, new FakeSshRemoteExecutor(), vault),
            TestPlatformHelpers.CreateFirewallApply(db, new FakeSshRemoteExecutor(), vault),
            new AdGuardSyncService(
                db,
                new ServiceCollection().AddHttpClient().BuildServiceProvider().GetRequiredService<IHttpClientFactory>(),
                secrets,
                new AuditService(db),
                syncRuns,
                new ConnectionTargetResolver(db, new AuditService(db))),
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
    public async Task ReconcileAsync_persists_awaiting_confirmation_when_adguard_deletes_pending()
    {
        var options = new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        await using var db = new HashiDbContext(options);
        db.AppSettings.Add(new AppSettingsEntity { RootDomain = "example.com" });

        var rootKey = new byte[32];
        var connectionId = await AddAdGuardConnectionAsync(db, rootKey);
        db.AdGuardRewrites.Add(new AdGuardRewriteEntity
        {
            ConnectionId = connectionId,
            Domain = "stale.example.com",
            Answer = "10.0.0.99",
            ManagedByHashi = true,
            Source = AdGuardRewriteSourceNames.Topology,
        });
        await db.SaveChangesAsync();

        var vault = new VaultSessionState();
        vault.Unlock(rootKey);
        var secrets = new SecretRecordService(db, vault, new ServiceSyncVaultState());
        var syncRuns = new SyncRunService(db);
        var handler = new FakeAdGuardHandler("""{"rewrites":[{"domain":"stale.example.com","answer":"10.0.0.99","id":"remote-1"}]}""");
        var orchestrator = new SyncOrchestratorService(
            db,
            new DnsConnectionService(db, new TestDnsProviderFactory(), secrets, new AuditService(db)),
            TestPlatformHelpers.CreateTraefikPlatform(db, vault),
            TestPlatformHelpers.CreateTraefikSync(db, new FakeSshRemoteExecutor(), vault),
            TestPlatformHelpers.CreateFirewallApply(db, new FakeSshRemoteExecutor(), vault),
            new AdGuardSyncService(
                db,
                new FakeHttpClientFactory(handler),
                secrets,
                new AuditService(db),
                syncRuns,
                new ConnectionTargetResolver(db, new AuditService(db))),
            syncRuns,
            new AppSettingsService(db),
            new AuditService(db));

        var result = await orchestrator.ReconcileAsync();

        Assert.True(result.Succeeded);
        var run = await db.SyncRuns.Include(x => x.Diffs).SingleAsync(x => x.Id == result.RunId);
        Assert.Equal(SyncRunStatusNames.AwaitingConfirmation, run.Status);
        Assert.Contains(run.Diffs, d =>
            d.ResourceType == "adguard-rewrite" &&
            d.ResourceKey == "stale.example.com" &&
            d.ChangeKind == nameof(ProviderResultKind.Deleted));
        Assert.Equal(0, handler.DeleteCalls);
        Assert.True(await db.AdGuardRewrites.AnyAsync(x => x.Domain == "stale.example.com"));
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

    private static async Task<Guid> AddAdGuardConnectionAsync(HashiDbContext db, byte[] rootKey)
    {
        var secretId = Guid.NewGuid();
        var dek = new byte[32];
        db.SecretRecords.Add(new SecretRecordEntity
        {
            Id = secretId,
            Purpose = SecretPurposeMapping.ToName(SecretPurpose.AdGuardCredential),
            Label = "AdGuard",
            AdminWrappedDekBlob = AesGcmCipher.Encrypt(dek, rootKey).ToBlob(),
            CiphertextBlob = AesGcmCipher.Encrypt(Encoding.UTF8.GetBytes("""{"password":"test"}"""), dek).ToBlob(),
        });

        var connectionId = Guid.NewGuid();
        db.AdGuardConnections.Add(new AdGuardConnectionEntity
        {
            Id = connectionId,
            Name = "home",
            BaseUrl = "http://adguard.test",
            PasswordSecretId = secretId,
            Enabled = true,
        });
        await db.SaveChangesAsync();
        return connectionId;
    }

    private sealed class FakeHttpClientFactory(HttpMessageHandler handler) : IHttpClientFactory
    {
        public HttpClient CreateClient(string name) => new(handler, disposeHandler: false);
    }

    private sealed class FakeAdGuardHandler(string rewriteListJson) : HttpMessageHandler
    {
        public int DeleteCalls { get; private set; }

        protected override Task<HttpResponseMessage> SendAsync(HttpRequestMessage request, CancellationToken cancellationToken)
        {
            if (request.RequestUri?.AbsolutePath.EndsWith("/control/rewrite/list", StringComparison.Ordinal) == true)
            {
                return Task.FromResult(JsonResponse(rewriteListJson));
            }

            if (request.RequestUri?.AbsolutePath.EndsWith("/control/rewrite/delete", StringComparison.Ordinal) == true)
            {
                DeleteCalls++;
                return Task.FromResult(JsonResponse("{}"));
            }

            return Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK)
            {
                Content = new StringContent("{}", Encoding.UTF8, "application/json"),
            });
        }

        private static HttpResponseMessage JsonResponse(string json) => new(HttpStatusCode.OK)
        {
            Content = new StringContent(json, Encoding.UTF8, "application/json"),
        };
    }
}
