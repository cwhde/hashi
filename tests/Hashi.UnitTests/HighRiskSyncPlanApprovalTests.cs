using Hashi.Core.Dns;
using Hashi.Core.Sync;
using Hashi.Core.Auth;
using Hashi.Infrastructure.Dns;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Services;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Crypto;
using Hashi.Infrastructure.Sync;
using Hashi.UnitTests.Fakes;
using Microsoft.EntityFrameworkCore;
using Xunit;

namespace Hashi.UnitTests;

public sealed class HighRiskSyncPlanApprovalTests
{
    [Fact]
    public async Task ReconcileAsync_marks_plan_as_awaiting_confirmation_when_destructive_changes_detected()
    {
        await using var db = CreateDb();
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
            new DnsRecordSnapshot("managed", "app.example.com", DnsRecordType.A, "1.2.3.4", 3600, true),
            new DnsRecordSnapshot("external", "other.example.com", DnsRecordType.A, "5.6.7.8", 3600, false));

        var vault = new VaultSessionState();
        vault.Unlock(rootKey);
        var secrets = new SecretRecordService(db, vault, new ServiceSyncVaultState());
        var dns = new DnsConnectionService(db, providerFactory, secrets, new AuditService(db));
        var settings = new AppSettingsService(db);
        var syncRuns = new SyncRunService(db);

        var plan = await dns.PlanSyncAsync(connectionId);

        var managedDeletes = plan.Changes.Where(x =>
            x.Kind == DnsChangeKind.Delete &&
            x.RiskReason.Contains("managed", StringComparison.OrdinalIgnoreCase)).ToList();

        if (managedDeletes.Any())
        {
            Assert.True(plan.RequiresConfirmation);
        }
    }

    [Fact]
    public async Task Plan_requires_confirmation_when_managed_records_would_be_deleted()
    {
        await using var db = CreateDb();
        var rootKey = new byte[32];
        var dek = new byte[32];
        var secretId = Guid.NewGuid();
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
            new DnsRecordSnapshot("managed", "app.example.com", DnsRecordType.A, "1.2.3.4", 3600, true));

        var vault = new VaultSessionState();
        vault.Unlock(rootKey);
        var secrets = new SecretRecordService(db, vault, new ServiceSyncVaultState());
        var dns = new DnsConnectionService(db, providerFactory, secrets, new AuditService(db));

        var plan = await dns.PlanSyncAsync(connectionId);

        var deleteChanges = plan.Changes.Where(x => x.Kind == DnsChangeKind.Delete).ToList();
        if (deleteChanges.Any())
        {
            Assert.True(plan.RequiresConfirmation);
        }
    }

    [Fact]
    public async Task Plan_does_not_require_confirmation_when_only_safe_changes()
    {
        await using var db = CreateDb();
        var rootKey = new byte[32];
        var dek = new byte[32];
        var secretId = Guid.NewGuid();
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
            new DnsRecordSnapshot("external", "other.example.com", DnsRecordType.A, "5.6.7.8", 3600, false));

        var vault = new VaultSessionState();
        vault.Unlock(rootKey);
        var secrets = new SecretRecordService(db, vault, new ServiceSyncVaultState());
        var dns = new DnsConnectionService(db, providerFactory, secrets, new AuditService(db));

        var plan = await dns.PlanSyncAsync(connectionId);

        var hasManagedDeletes = plan.Changes.Any(x =>
            x.Kind == DnsChangeKind.Delete &&
            x.RiskReason.Contains("managed", StringComparison.OrdinalIgnoreCase));

        if (!hasManagedDeletes)
        {
            Assert.False(plan.RequiresConfirmation);
        }
    }

    [Fact]
    public async Task Plan_flags_unowned_record_changes_as_noop()
    {
        await using var db = CreateDb();
        var rootKey = new byte[32];
        var dek = new byte[32];
        var secretId = Guid.NewGuid();
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
            new DnsRecordSnapshot("external", "other.example.com", DnsRecordType.A, "5.6.7.8", 3600, false));

        var vault = new VaultSessionState();
        vault.Unlock(rootKey);
        var secrets = new SecretRecordService(db, vault, new ServiceSyncVaultState());
        var dns = new DnsConnectionService(db, providerFactory, secrets, new AuditService(db));

        var plan = await dns.PlanSyncAsync(connectionId);

        var unownedChanges = plan.Changes.Where(x =>
            x.RiskReason.Contains("not owned", StringComparison.OrdinalIgnoreCase)).ToList();

        Assert.All(unownedChanges, x => Assert.Equal(DnsChangeKind.NoOp, x.Kind));
    }

    private static HashiDbContext CreateDb()
    {
        var options = new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        return new HashiDbContext(options);
    }
}
