using System.Net;
using System.Text;
using Hashi.Contracts.Api;
using Hashi.Core.Auth;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Platform;
using Hashi.Infrastructure.Services;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Configuration;
using Microsoft.Extensions.Logging.Abstractions;
using Xunit;

namespace Hashi.UnitTests;

public sealed class GeoIpUpdateServiceTests
{
    [Fact]
    public async Task UpdateAsync_stores_maxmind_license_key_as_service_sync_secret()
    {
        await using var db = CreateDb();
        var vault = UnlockedVault();
        var serviceSync = ReadyServiceSyncVault();
        var secrets = new SecretRecordService(db, vault, serviceSync);
        var lookup = CreateLookup(Path.Combine(Path.GetTempPath(), Guid.NewGuid().ToString("N")));
        var service = new GeoIpSettingsService(db, new AppSettingsService(db), secrets, vault, lookup);

        var response = await service.UpdateAsync(new GeoIpSettingsRequest(
            Enabled: true,
            AccountId: "123456",
            LicenseKey: "license-key",
            UpdateIntervalHours: 24));

        Assert.True(response.Enabled);
        Assert.True(response.HasLicenseKey);
        Assert.Equal("123456", response.AccountId);
        Assert.Null(response.LastUpdateMessage);

        var settings = await db.AppSettings.SingleAsync();
        Assert.NotNull(settings.GeoIpLicenseKeySecretId);
        var stored = await db.SecretRecords.SingleAsync(x => x.Id == settings.GeoIpLicenseKeySecretId);
        Assert.Equal(SecretPurposeNames.MaxMindLicenseKey, stored.Purpose);
        Assert.True(stored.IsServiceSyncEligible);
        Assert.Equal("license-key", Encoding.UTF8.GetString((await secrets.DecryptForServiceSyncAsync(stored.Id))!));
    }

    [Fact]
    public async Task RunUpdateAsync_downloads_databases_records_metadata_and_reloads_lookup()
    {
        await using var db = CreateDb();
        var dataPath = CreateTempDataPath();
        var vault = UnlockedVault();
        var serviceSync = ReadyServiceSyncVault();
        var secrets = new SecretRecordService(db, vault, serviceSync);
        var lookup = CreateLookup(dataPath);
        Assert.False(lookup.IsAvailable);

        var settingsService = new GeoIpSettingsService(db, new AppSettingsService(db), secrets, vault, lookup);
        await settingsService.UpdateAsync(new GeoIpSettingsRequest(
            Enabled: true,
            AccountId: "123456",
            LicenseKey: "license-key",
            UpdateIntervalHours: 24));

        var lockedSecrets = new SecretRecordService(db, new VaultSessionState(), serviceSync);
        var updater = CreateUpdater(db, dataPath, lockedSecrets, lookup, new FakeGeoIpDownloader());

        var result = await updater.RunUpdateAsync();

        Assert.True(result.Succeeded);
        Assert.Equal(GeoIpUpdateStatusNames.Succeeded, result.Status);
        Assert.Equal(3, result.Databases.Count);
        Assert.All(result.Databases, item => Assert.Equal(GeoIpUpdateStatusNames.Succeeded, item.Status));
        Assert.True(File.Exists(Path.Combine(dataPath, "geoip", "GeoLite2-City.mmdb")));
        Assert.True(lookup.IsAvailable);

        var settings = await db.AppSettings.SingleAsync();
        Assert.Equal(GeoIpUpdateStatusNames.Succeeded, settings.GeoIpLastUpdateStatus);
        Assert.NotNull(settings.GeoIpLastUpdateAtUtc);
        Assert.NotNull(settings.GeoIpNextUpdateAtUtc);
        Assert.Equal(3, await db.GeoIpDatabases.CountAsync());
    }

    [Fact]
    public async Task RunUpdateAsync_records_failure_when_a_database_download_fails()
    {
        await using var db = CreateDb();
        var dataPath = CreateTempDataPath();
        var vault = UnlockedVault();
        var serviceSync = ReadyServiceSyncVault();
        var secrets = new SecretRecordService(db, vault, serviceSync);
        var lookup = CreateLookup(dataPath);
        var settingsService = new GeoIpSettingsService(db, new AppSettingsService(db), secrets, vault, lookup);
        await settingsService.UpdateAsync(new GeoIpSettingsRequest(
            Enabled: true,
            AccountId: "123456",
            LicenseKey: "license-key",
            UpdateIntervalHours: 24));
        var updater = CreateUpdater(
            db,
            dataPath,
            new SecretRecordService(db, new VaultSessionState(), serviceSync),
            lookup,
            new FakeGeoIpDownloader(failEditionId: GeoIpDatabaseEdition.Asn.EditionId));

        var result = await updater.RunUpdateAsync();

        Assert.False(result.Succeeded);
        Assert.Equal(GeoIpUpdateStatusNames.Failed, result.Status);
        Assert.Contains(GeoIpDatabaseEdition.Asn.EditionId, result.Message, StringComparison.Ordinal);
        Assert.Contains(result.Databases, x => x.EditionId == GeoIpDatabaseEdition.Asn.EditionId && x.Status == GeoIpUpdateStatusNames.Failed);
        Assert.Equal(GeoIpUpdateStatusNames.Failed, (await db.AppSettings.SingleAsync()).GeoIpLastUpdateStatus);
    }

    private static GeoIpUpdateService CreateUpdater(
        HashiDbContext db,
        string dataPath,
        SecretRecordService secrets,
        GeoIpLookupService lookup,
        IGeoIpDatabaseDownloader downloader)
    {
        var configuration = new ConfigurationBuilder()
            .AddInMemoryCollection(new Dictionary<string, string?>
            {
                ["Hashi:DataPath"] = dataPath,
            })
            .Build();
        return new GeoIpUpdateService(
            db,
            new AppSettingsService(db),
            secrets,
            downloader,
            lookup,
            configuration);
    }

    private static GeoIpLookupService CreateLookup(string dataPath)
    {
        var configuration = new ConfigurationBuilder()
            .AddInMemoryCollection(new Dictionary<string, string?>
            {
                ["Hashi:DataPath"] = dataPath,
            })
            .Build();
        return new GeoIpLookupService(
            configuration,
            NullLogger<GeoIpLookupService>.Instance,
            new ExistingFileReaderFactory());
    }

    private static HashiDbContext CreateDb()
        => new(new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options);

    private static VaultSessionState UnlockedVault()
    {
        var vault = new VaultSessionState();
        vault.Unlock(new byte[32]);
        return vault;
    }

    private static ServiceSyncVaultState ReadyServiceSyncVault()
    {
        var serviceSync = new ServiceSyncVaultState();
        serviceSync.Initialize([1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32]);
        return serviceSync;
    }

    private static string CreateTempDataPath()
    {
        var path = Path.Combine(Path.GetTempPath(), "hashi-geoip-tests", Guid.NewGuid().ToString("N"));
        Directory.CreateDirectory(path);
        return path;
    }

    private sealed class FakeGeoIpDownloader(string? failEditionId = null) : IGeoIpDatabaseDownloader
    {
        public async Task<GeoIpDownloadResult> DownloadAsync(
            GeoIpDatabaseEdition edition,
            string destinationPath,
            string accountId,
            string licenseKey,
            CancellationToken cancellationToken = default)
        {
            Assert.Equal("123456", accountId);
            Assert.Equal("license-key", licenseKey);
            if (edition.EditionId == failEditionId)
            {
                throw new HttpRequestException("download failed");
            }

            var bytes = Encoding.UTF8.GetBytes($"database:{edition.EditionId}");
            await File.WriteAllBytesAsync(destinationPath, bytes, cancellationToken);
            return new GeoIpDownloadResult(DateTimeOffset.UtcNow, bytes.Length, Convert.ToHexString(bytes).ToLowerInvariant());
        }
    }

    private sealed class ExistingFileReaderFactory : IGeoIpDatabaseReaderFactory
    {
        public IGeoIpDatabaseReader Open(string path)
        {
            if (!File.Exists(path))
            {
                throw new FileNotFoundException("GeoIP database not found.", path);
            }

            return new FakeGeoIpReader();
        }
    }

    private sealed class FakeGeoIpReader : IGeoIpDatabaseReader
    {
        public GeoIpLookup LookupCity(IPAddress address) => new("US", "CA", null);

        public string? LookupAsn(IPAddress address) => "AS64512";

        public void Dispose()
        {
        }
    }
}
