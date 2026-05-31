using System.Text;
using Hashi.Contracts.Api;
using Hashi.Core.Auth;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Platform;
using Hashi.Infrastructure.Services;
using Microsoft.EntityFrameworkCore;
using Xunit;

namespace Hashi.UnitTests;

public sealed class CertificateSetupServiceTests
{
    [Fact]
    public async Task SaveAsync_with_locked_vault_saves_non_secret_settings_without_persisting_eab()
    {
        await using var db = CreateDb();
        SeedDnsConnection(db);
        var service = CreateService(db, new VaultSessionState());

        var result = await service.SaveAsync(CreateRequest());

        Assert.False(result.Saved);
        Assert.Contains("Unlock the vault", result.Error);
        var settings = await db.AppSettings.SingleAsync();
        Assert.Equal("admin@example.com", settings.AcmeEmail);
        Assert.Equal(45, settings.DnsChallengeDelaySeconds);
        Assert.Null(settings.AcmeEabSecretId);
        Assert.Empty(await db.SecretRecords.ToListAsync());
        Assert.Empty(await db.SetupStates.ToListAsync());
    }

    [Fact]
    public async Task SaveAsync_with_unlocked_vault_stores_eab_as_secret()
    {
        await using var db = CreateDb();
        SeedDnsConnection(db);
        var vault = new VaultSessionState();
        vault.Unlock(new byte[32]);
        var service = CreateService(db, vault);

        var result = await service.SaveAsync(CreateRequest());

        Assert.True(result.Saved);
        var settings = await db.AppSettings.SingleAsync();
        Assert.NotNull(settings.AcmeEabSecretId);
        var secret = await db.SecretRecords.SingleAsync();
        Assert.Equal(SecretPurposeNames.AcmeEab, secret.Purpose);
        Assert.True(secret.IsServiceSyncEligible);

        var secrets = new SecretRecordService(db, vault, new ServiceSyncVaultState());
        var plaintext = await secrets.DecryptForAdminAsync(settings.AcmeEabSecretId.Value);
        Assert.NotNull(plaintext);
        Assert.Contains("eab-key", Encoding.UTF8.GetString(plaintext));
        Assert.Contains("eab-hmac", Encoding.UTF8.GetString(plaintext));
    }

    private static CertificateSetupRequest CreateRequest() => new(
        AcmeEmail: "admin@example.com",
        EabKeyId: "eab-key",
        EabHmac: "eab-hmac",
        DnsChallengeDelaySeconds: 45,
        Resolvers: ["1.1.1.1:53"]);

    private static CertificateSetupService CreateService(HashiDbContext db, VaultSessionState vault)
    {
        var serviceSync = new ServiceSyncVaultState();
        var secrets = new SecretRecordService(db, vault, serviceSync);
        return new CertificateSetupService(db, new AppSettingsService(db), secrets, vault, new AuditService(db));
    }

    private static HashiDbContext CreateDb()
        => new(new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options);

    private static void SeedDnsConnection(HashiDbContext db)
    {
        db.Connections.Add(new ConnectionEntity
        {
            Name = "dns",
            Type = "dns",
            Enabled = true,
        });
        db.SaveChanges();
    }
}
