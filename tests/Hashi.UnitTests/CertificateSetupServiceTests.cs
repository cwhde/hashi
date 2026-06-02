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
        var serviceSync = ReadyServiceSyncVault();
        var dnsConnectionId = await SeedDnsConnectionAsync(db, serviceSync);
        var service = CreateService(db, new VaultSessionState(), serviceSync);

        var result = await service.SaveAsync(CreateRequest(dnsConnectionId));

        Assert.False(result.Saved);
        Assert.Contains("Unlock the vault", result.Error);
        var settings = await db.AppSettings.SingleAsync();
        Assert.Equal("admin@example.com", settings.AcmeEmail);
        Assert.Equal(dnsConnectionId, settings.AcmeDnsProviderConnectionId);
        Assert.Equal(45, settings.DnsChallengeDelaySeconds);
        Assert.Null(settings.AcmeEabSecretId);
        Assert.Empty(await db.SecretRecords.Where(x => x.Purpose == SecretPurposeNames.AcmeEab).ToListAsync());
        Assert.Empty(await db.SetupStates.ToListAsync());
    }

    [Fact]
    public async Task SaveAsync_with_unlocked_vault_stores_eab_as_secret()
    {
        await using var db = CreateDb();
        var serviceSync = ReadyServiceSyncVault();
        var dnsConnectionId = await SeedDnsConnectionAsync(db, serviceSync);
        var vault = new VaultSessionState();
        vault.Unlock(new byte[32]);
        var service = CreateService(db, vault, serviceSync);

        var result = await service.SaveAsync(CreateRequest(dnsConnectionId));

        Assert.True(result.Saved);
        var settings = await db.AppSettings.SingleAsync();
        Assert.Equal(dnsConnectionId, settings.AcmeDnsProviderConnectionId);
        Assert.NotNull(settings.AcmeEabSecretId);
        var secret = await db.SecretRecords.SingleAsync(x => x.Id == settings.AcmeEabSecretId);
        Assert.Equal(SecretPurposeNames.AcmeEab, secret.Purpose);
        Assert.True(secret.IsServiceSyncEligible);

        var secrets = new SecretRecordService(db, vault, serviceSync);
        var plaintext = await secrets.DecryptForAdminAsync(settings.AcmeEabSecretId.Value);
        Assert.NotNull(plaintext);
        Assert.Contains("eab-key", Encoding.UTF8.GetString(plaintext));
        Assert.Contains("eab-hmac", Encoding.UTF8.GetString(plaintext));
    }

    [Fact]
    public async Task ValidateAsync_accepts_enabled_dns_provider_connection_type()
    {
        await using var db = CreateDb();
        var serviceSync = ReadyServiceSyncVault();
        var dnsConnectionId = await SeedDnsConnectionAsync(db, serviceSync);
        var service = CreateService(db, new VaultSessionState(), serviceSync);

        var result = await service.ValidateAsync(CreateRequest(dnsConnectionId));

        Assert.True(result.IsValid, string.Join("; ", result.Errors));
    }

    [Fact]
    public async Task ValidateAsync_rejects_legacy_dns_connection_type()
    {
        await using var db = CreateDb();
        var serviceSync = ReadyServiceSyncVault();
        var legacyId = Guid.NewGuid();
        db.Connections.Add(new ConnectionEntity
        {
            Id = legacyId,
            Name = "legacy dns",
            Type = "dns",
            Enabled = true,
            SettingsJson = """{"provider":"hetzner","zoneName":"example.com","defaultTtl":3600}""",
        });
        await db.SaveChangesAsync();
        var service = CreateService(db, new VaultSessionState(), serviceSync);

        var result = await service.ValidateAsync(CreateRequest(legacyId));

        Assert.False(result.IsValid);
        Assert.Contains(result.Errors, x => x.Contains("not found", StringComparison.OrdinalIgnoreCase));
    }

    [Fact]
    public async Task ValidateAsync_rejects_missing_disabled_and_unsupported_bindings()
    {
        await using var db = CreateDb();
        var serviceSync = ReadyServiceSyncVault();
        var disabledId = await SeedDnsConnectionAsync(db, serviceSync, enabled: false);
        var unsupportedId = await SeedDnsConnectionAsync(db, serviceSync, provider: "cloudflare");
        var service = CreateService(db, new VaultSessionState(), serviceSync);

        var missing = await service.ValidateAsync(CreateRequest(null));
        var disabled = await service.ValidateAsync(CreateRequest(disabledId));
        var unsupported = await service.ValidateAsync(CreateRequest(unsupportedId));

        Assert.Contains(missing.Errors, x => x.Contains("Select an enabled DNS provider", StringComparison.OrdinalIgnoreCase));
        Assert.Contains(disabled.Errors, x => x.Contains("disabled", StringComparison.OrdinalIgnoreCase));
        Assert.Contains(unsupported.Errors, x => x.Contains("not supported", StringComparison.OrdinalIgnoreCase));
    }

    [Fact]
    public async Task ValidateAsync_rejects_provider_without_service_sync_credentials()
    {
        await using var db = CreateDb();
        var serviceSync = new ServiceSyncVaultState();
        var dnsConnectionId = await SeedDnsConnectionAsync(db, serviceSync);
        var service = CreateService(db, new VaultSessionState(), serviceSync);

        var result = await service.ValidateAsync(CreateRequest(dnsConnectionId));

        Assert.False(result.IsValid);
        Assert.Contains(result.Errors, x => x.Contains("service sync", StringComparison.OrdinalIgnoreCase));
    }

    private static CertificateSetupRequest CreateRequest(Guid? dnsProviderConnectionId) => new(
        AcmeEmail: "admin@example.com",
        EabKeyId: "eab-key",
        EabHmac: "eab-hmac",
        DnsChallengeDelaySeconds: 45,
        Resolvers: ["1.1.1.1:53"],
        DnsProviderConnectionId: dnsProviderConnectionId);

    private static CertificateSetupService CreateService(
        HashiDbContext db,
        VaultSessionState vault,
        ServiceSyncVaultState serviceSync)
    {
        var secrets = new SecretRecordService(db, vault, serviceSync);
        return new CertificateSetupService(db, new AppSettingsService(db), secrets, vault, new AuditService(db));
    }

    private static HashiDbContext CreateDb()
        => new(new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options);

    private static ServiceSyncVaultState ReadyServiceSyncVault()
    {
        var serviceSync = new ServiceSyncVaultState();
        serviceSync.Initialize([1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32]);
        return serviceSync;
    }

    private static async Task<Guid> SeedDnsConnectionAsync(
        HashiDbContext db,
        ServiceSyncVaultState serviceSync,
        bool enabled = true,
        string provider = DnsProviderTypeNames.Hetzner)
    {
        var vault = new VaultSessionState();
        vault.Unlock(new byte[32]);
        var secrets = new SecretRecordService(db, vault, serviceSync);
        var token = await secrets.StoreAsync(
            SecretPurpose.DnsProviderToken,
            "DNS provider",
            Encoding.UTF8.GetBytes("dns-token"),
            serviceSyncEligible: true);
        var connectionId = Guid.NewGuid();
        db.Connections.Add(new ConnectionEntity
        {
            Id = connectionId,
            Name = "dns",
            Type = ConnectionTypeNames.DnsProvider,
            Enabled = enabled,
            SecretId = token.Id,
            SettingsJson = $$"""{"provider":"{{provider}}","zoneName":"example.com","defaultTtl":3600}""",
        });
        await db.SaveChangesAsync();
        return connectionId;
    }
}
