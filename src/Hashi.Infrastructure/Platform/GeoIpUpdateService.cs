using System.Formats.Tar;
using System.IO.Compression;
using System.Net.Http.Headers;
using System.Security.Cryptography;
using System.Text;
using Hashi.Contracts.Api;
using Hashi.Core.Auth;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Services;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Configuration;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Hosting;
using Microsoft.Extensions.Logging;

namespace Hashi.Infrastructure.Platform;

public sealed record GeoIpDatabaseEdition(string EditionId, string FileName)
{
    public static readonly GeoIpDatabaseEdition City = new("GeoLite2-City", "GeoLite2-City.mmdb");
    public static readonly GeoIpDatabaseEdition Country = new("GeoLite2-Country", "GeoLite2-Country.mmdb");
    public static readonly GeoIpDatabaseEdition Asn = new("GeoLite2-ASN", "GeoLite2-ASN.mmdb");

    public static readonly IReadOnlyList<GeoIpDatabaseEdition> All = [City, Country, Asn];
}

public sealed record GeoIpDownloadResult(DateTimeOffset? LastModifiedUtc, long SizeBytes, string ContentHash);

public interface IGeoIpDatabaseDownloader
{
    Task<GeoIpDownloadResult> DownloadAsync(
        GeoIpDatabaseEdition edition,
        string destinationPath,
        string accountId,
        string licenseKey,
        CancellationToken cancellationToken = default);
}

public sealed class GeoIpSettingsService(
    HashiDbContext db,
    AppSettingsService settings,
    SecretRecordService secrets,
    VaultSessionState vault,
    GeoIpLookupService lookup)
{
    public async Task<GeoIpSettingsResponse> GetAsync(CancellationToken cancellationToken = default)
    {
        var s = await settings.GetOrCreateAsync(cancellationToken);
        return new GeoIpSettingsResponse(
            s.GeoIpEnabled,
            s.GeoIpAccountId,
            s.GeoIpLicenseKeySecretId is not null,
            s.GeoIpLicenseKeySecretId,
            s.GeoIpUpdateIntervalHours,
            s.GeoIpLastUpdateStatus,
            s.GeoIpLastUpdateMessage,
            s.GeoIpLastUpdateAtUtc,
            s.GeoIpNextUpdateAtUtc,
            lookup.IsAvailable,
            await ListDatabasesAsync(cancellationToken),
            s.UpdatedAtUtc);
    }

    public async Task<GeoIpSettingsResponse> UpdateAsync(
        GeoIpSettingsRequest request,
        CancellationToken cancellationToken = default)
    {
        var s = await settings.GetOrCreateAsync(cancellationToken);
        if (request.AccountId is not null)
        {
            s.GeoIpAccountId = string.IsNullOrWhiteSpace(request.AccountId) ? null : request.AccountId.Trim();
        }

        if (request.UpdateIntervalHours is int interval)
        {
            s.GeoIpUpdateIntervalHours = Math.Clamp(interval, 12, 168);
        }

        if (!string.IsNullOrWhiteSpace(request.LicenseKey))
        {
            if (!vault.IsUnlocked)
            {
                throw new InvalidOperationException("Unlock the vault before saving the MaxMind license key.");
            }

            var secret = await secrets.StoreAsync(
                SecretPurpose.MaxMindLicenseKey,
                "MaxMind GeoLite2 license key",
                Encoding.UTF8.GetBytes(request.LicenseKey.Trim()),
                cancellationToken,
                serviceSyncEligible: RuntimeSecretEligibility.IsRuntimePurpose(SecretPurpose.MaxMindLicenseKey));
            s.GeoIpLicenseKeySecretId = secret.Id;
        }

        if (request.Enabled is bool enabled)
        {
            if (enabled && string.IsNullOrWhiteSpace(s.GeoIpAccountId))
            {
                throw new InvalidOperationException("MaxMind account ID is required before enabling GeoIP updates.");
            }

            if (enabled && s.GeoIpLicenseKeySecretId is null)
            {
                throw new InvalidOperationException("MaxMind license key is required before enabling GeoIP updates.");
            }

            s.GeoIpEnabled = enabled;
            if (!enabled)
            {
                s.GeoIpLastUpdateStatus = GeoIpUpdateStatusNames.Disabled;
                s.GeoIpNextUpdateAtUtc = null;
            }
            else
            {
                s.GeoIpNextUpdateAtUtc ??= DateTimeOffset.UtcNow;
            }
        }

        s.UpdatedAtUtc = DateTimeOffset.UtcNow;
        await settings.SaveAsync(cancellationToken);
        return await GetAsync(cancellationToken);
    }

    internal async Task<IReadOnlyList<GeoIpDatabaseResponse>> ListDatabasesAsync(CancellationToken cancellationToken)
    {
        var stored = await db.GeoIpDatabases.AsNoTracking().ToDictionaryAsync(x => x.EditionId, cancellationToken);
        return GeoIpDatabaseEdition.All.Select(edition =>
        {
            stored.TryGetValue(edition.EditionId, out var entity);
            return entity is null
                ? new GeoIpDatabaseResponse(edition.EditionId, edition.FileName, GeoIpUpdateStatusNames.NeverRun, null, null, null, null, null)
                : ToResponse(entity);
        }).ToList();
    }

    internal static GeoIpDatabaseResponse ToResponse(GeoIpDatabaseEntity entity)
        => new(
            entity.EditionId,
            entity.FileName,
            entity.Status,
            entity.LastDownloadedAtUtc,
            entity.LastModifiedUtc,
            entity.SizeBytes,
            entity.ContentHash,
            entity.Error);
}

public sealed class GeoIpUpdateService(
    HashiDbContext db,
    AppSettingsService settings,
    SecretRecordService secrets,
    IGeoIpDatabaseDownloader downloader,
    GeoIpLookupService lookup,
    IConfiguration configuration)
{
    private string DataPath => configuration["Hashi:DataPath"] is { Length: > 0 } path
        ? Path.Combine(path, "geoip")
        : "/data/geoip";

    public async Task<GeoIpUpdateResponse> RunUpdateAsync(CancellationToken cancellationToken = default)
    {
        var s = await settings.GetOrCreateAsync(cancellationToken);
        if (!s.GeoIpEnabled)
        {
            s.GeoIpLastUpdateStatus = GeoIpUpdateStatusNames.Disabled;
            s.GeoIpLastUpdateMessage = "GeoIP updates are disabled.";
            s.GeoIpNextUpdateAtUtc = null;
            await settings.SaveAsync(cancellationToken);
            return new GeoIpUpdateResponse(false, s.GeoIpLastUpdateStatus, s.GeoIpLastUpdateMessage, await ListDatabasesAsync(cancellationToken));
        }

        if (string.IsNullOrWhiteSpace(s.GeoIpAccountId) || s.GeoIpLicenseKeySecretId is null)
        {
            return await RecordFailureAsync(s, "MaxMind account ID and license key must be configured.", cancellationToken);
        }

        var licenseBytes = await secrets.DecryptForPurposeAsync(s.GeoIpLicenseKeySecretId.Value, cancellationToken);
        if (licenseBytes is null)
        {
            return await RecordFailureAsync(s, "MaxMind license key is unavailable. Unlock the vault or configure service-sync vault access.", cancellationToken);
        }

        var licenseKey = Encoding.UTF8.GetString(licenseBytes).Trim();
        CryptographicOperations.ZeroMemory(licenseBytes);
        if (string.IsNullOrWhiteSpace(licenseKey))
        {
            return await RecordFailureAsync(s, "MaxMind license key is empty.", cancellationToken);
        }

        Directory.CreateDirectory(DataPath);
        s.GeoIpLastUpdateStatus = GeoIpUpdateStatusNames.Running;
        s.GeoIpLastUpdateMessage = "GeoIP update is running.";
        await settings.SaveAsync(cancellationToken);

        var failures = new List<string>();
        var updated = 0;
        foreach (var edition in GeoIpDatabaseEdition.All)
        {
            cancellationToken.ThrowIfCancellationRequested();
            var finalPath = Path.Combine(DataPath, edition.FileName);
            var tempPath = Path.Combine(DataPath, $".{edition.FileName}.{Guid.NewGuid():N}.tmp");
            var metadata = await GetOrCreateMetadataAsync(edition, finalPath, cancellationToken);
            metadata.Status = GeoIpUpdateStatusNames.Running;
            metadata.Error = null;
            metadata.UpdatedAtUtc = DateTimeOffset.UtcNow;
            await db.SaveChangesAsync(cancellationToken);

            try
            {
                var result = await downloader.DownloadAsync(
                    edition,
                    tempPath,
                    s.GeoIpAccountId,
                    licenseKey,
                    cancellationToken);
                AtomicReplace(tempPath, finalPath);
                metadata.Status = GeoIpUpdateStatusNames.Succeeded;
                metadata.LastDownloadedAtUtc = DateTimeOffset.UtcNow;
                metadata.LastModifiedUtc = result.LastModifiedUtc;
                metadata.SizeBytes = result.SizeBytes;
                metadata.ContentHash = result.ContentHash;
                metadata.Error = null;
                metadata.UpdatedAtUtc = DateTimeOffset.UtcNow;
                updated++;
            }
            catch (Exception ex) when (ex is not OperationCanceledException)
            {
                TryDelete(tempPath);
                metadata.Status = GeoIpUpdateStatusNames.Failed;
                metadata.Error = ex.Message;
                metadata.UpdatedAtUtc = DateTimeOffset.UtcNow;
                failures.Add($"{edition.EditionId}: {ex.Message}");
            }

            await db.SaveChangesAsync(cancellationToken);
        }

        var succeeded = failures.Count == 0;
        s.GeoIpLastUpdateStatus = succeeded ? GeoIpUpdateStatusNames.Succeeded : GeoIpUpdateStatusNames.Failed;
        s.GeoIpLastUpdateAtUtc = DateTimeOffset.UtcNow;
        s.GeoIpNextUpdateAtUtc = s.GeoIpLastUpdateAtUtc.Value.AddHours(Math.Clamp(s.GeoIpUpdateIntervalHours, 12, 168));
        s.GeoIpLastUpdateMessage = succeeded
            ? $"Updated {updated} GeoLite2 databases."
            : string.Join("; ", failures);
        s.UpdatedAtUtc = DateTimeOffset.UtcNow;
        await settings.SaveAsync(cancellationToken);
        if (updated > 0)
        {
            lookup.Reload();
        }

        return new GeoIpUpdateResponse(succeeded, s.GeoIpLastUpdateStatus, s.GeoIpLastUpdateMessage, await ListDatabasesAsync(cancellationToken));
    }

    private async Task<GeoIpDatabaseEntity> GetOrCreateMetadataAsync(
        GeoIpDatabaseEdition edition,
        string finalPath,
        CancellationToken cancellationToken)
    {
        var entity = await db.GeoIpDatabases.SingleOrDefaultAsync(x => x.EditionId == edition.EditionId, cancellationToken);
        if (entity is not null)
        {
            entity.FileName = edition.FileName;
            entity.Path = finalPath;
            return entity;
        }

        entity = new GeoIpDatabaseEntity
        {
            EditionId = edition.EditionId,
            FileName = edition.FileName,
            Path = finalPath,
        };
        db.GeoIpDatabases.Add(entity);
        return entity;
    }

    private async Task<GeoIpUpdateResponse> RecordFailureAsync(
        AppSettingsEntity settingsEntity,
        string message,
        CancellationToken cancellationToken)
    {
        settingsEntity.GeoIpLastUpdateStatus = GeoIpUpdateStatusNames.Failed;
        settingsEntity.GeoIpLastUpdateMessage = message;
        settingsEntity.GeoIpLastUpdateAtUtc = DateTimeOffset.UtcNow;
        settingsEntity.GeoIpNextUpdateAtUtc = settingsEntity.GeoIpLastUpdateAtUtc.Value.AddHours(Math.Clamp(settingsEntity.GeoIpUpdateIntervalHours, 12, 168));
        settingsEntity.UpdatedAtUtc = DateTimeOffset.UtcNow;
        await settings.SaveAsync(cancellationToken);
        return new GeoIpUpdateResponse(false, settingsEntity.GeoIpLastUpdateStatus, message, await ListDatabasesAsync(cancellationToken));
    }

    private async Task<IReadOnlyList<GeoIpDatabaseResponse>> ListDatabasesAsync(CancellationToken cancellationToken)
    {
        var stored = await db.GeoIpDatabases.AsNoTracking().ToDictionaryAsync(x => x.EditionId, cancellationToken);
        return GeoIpDatabaseEdition.All.Select(edition =>
        {
            stored.TryGetValue(edition.EditionId, out var entity);
            return entity is null
                ? new GeoIpDatabaseResponse(edition.EditionId, edition.FileName, GeoIpUpdateStatusNames.NeverRun, null, null, null, null, null)
                : GeoIpSettingsService.ToResponse(entity);
        }).ToList();
    }

    private static void AtomicReplace(string tempPath, string finalPath)
    {
        if (File.Exists(finalPath))
        {
            var backupPath = Path.Combine(Path.GetDirectoryName(finalPath)!, $".{Path.GetFileName(finalPath)}.{Guid.NewGuid():N}.bak");
            File.Replace(tempPath, finalPath, backupPath, ignoreMetadataErrors: true);
            TryDelete(backupPath);
            return;
        }

        File.Move(tempPath, finalPath);
    }

    private static void TryDelete(string path)
    {
        try
        {
            if (File.Exists(path))
            {
                File.Delete(path);
            }
        }
        catch
        {
        }
    }
}

public sealed class MaxMindGeoIpDatabaseDownloader(IHttpClientFactory httpClientFactory) : IGeoIpDatabaseDownloader
{
    public async Task<GeoIpDownloadResult> DownloadAsync(
        GeoIpDatabaseEdition edition,
        string destinationPath,
        string accountId,
        string licenseKey,
        CancellationToken cancellationToken = default)
    {
        var client = httpClientFactory.CreateClient("maxmind-geoip");
        using var request = new HttpRequestMessage(
            HttpMethod.Get,
            $"geoip/databases/{edition.EditionId}/download?suffix=tar.gz");
        request.Headers.Authorization = new AuthenticationHeaderValue(
            "Basic",
            Convert.ToBase64String(Encoding.ASCII.GetBytes($"{accountId}:{licenseKey}")));
        using var response = await client.SendAsync(request, HttpCompletionOption.ResponseHeadersRead, cancellationToken);
        response.EnsureSuccessStatusCode();

        await using var body = await response.Content.ReadAsStreamAsync(cancellationToken);
        await using var gzip = new GZipStream(body, CompressionMode.Decompress);
        await ExtractMmdbAsync(gzip, edition.FileName, destinationPath, cancellationToken);

        var size = new FileInfo(destinationPath).Length;
        await using var written = File.OpenRead(destinationPath);
        var hash = Convert.ToHexString(await SHA256.HashDataAsync(written, cancellationToken)).ToLowerInvariant();
        return new GeoIpDownloadResult(response.Content.Headers.LastModified, size, hash);
    }

    private static async Task ExtractMmdbAsync(
        Stream tarStream,
        string fileName,
        string destinationPath,
        CancellationToken cancellationToken)
    {
        await using var output = File.Create(destinationPath);
        using var reader = new TarReader(tarStream);
        TarEntry? entry;
        while ((entry = await reader.GetNextEntryAsync(copyData: false, cancellationToken)) is not null)
        {
            if (entry.DataStream is null)
            {
                continue;
            }

            if (!string.Equals(Path.GetFileName(entry.Name), fileName, StringComparison.Ordinal))
            {
                continue;
            }

            await entry.DataStream.CopyToAsync(output, cancellationToken);
            return;
        }

        throw new InvalidOperationException($"Downloaded archive did not contain {fileName}.");
    }
}

public sealed class GeoIpUpdateWorker(
    IServiceScopeFactory scopeFactory,
    ILogger<GeoIpUpdateWorker> logger) : BackgroundService
{
    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        while (!stoppingToken.IsCancellationRequested)
        {
            var delay = TimeSpan.FromMinutes(10);
            try
            {
                using var scope = scopeFactory.CreateScope();
                var settings = scope.ServiceProvider.GetRequiredService<AppSettingsService>();
                var appSettings = await settings.GetOrCreateAsync(stoppingToken);
                var intervalSeconds = Math.Clamp(appSettings.GeoIpUpdateIntervalHours, 12, 168) * 3600;
                delay = TimeSpan.FromSeconds(Math.Clamp(intervalSeconds / 4, 300, 3600));

                if (appSettings.GeoIpEnabled
                    && (appSettings.GeoIpNextUpdateAtUtc is null || appSettings.GeoIpNextUpdateAtUtc <= DateTimeOffset.UtcNow))
                {
                    var jobs = scope.ServiceProvider.GetRequiredService<BackgroundJobService>();
                    await jobs.BeginRunAsync(BackgroundJobKeys.GeoIpUpdate, stoppingToken);
                    var updater = scope.ServiceProvider.GetRequiredService<GeoIpUpdateService>();
                    var result = await updater.RunUpdateAsync(stoppingToken);
                    await jobs.CompleteRunAsync(
                        BackgroundJobKeys.GeoIpUpdate,
                        result.Succeeded,
                        result.Message,
                        result.Succeeded ? null : result.Message,
                        intervalSeconds,
                        stoppingToken);
                }
            }
            catch (Exception ex) when (ex is not OperationCanceledException)
            {
                logger.LogError(ex, "GeoIP update worker failed.");
                try
                {
                    using var scope = scopeFactory.CreateScope();
                    var jobs = scope.ServiceProvider.GetRequiredService<BackgroundJobService>();
                    await jobs.CompleteRunAsync(BackgroundJobKeys.GeoIpUpdate, false, null, ex.Message, 86400, stoppingToken);
                }
                catch
                {
                }
            }

            await Task.Delay(delay, stoppingToken);
        }
    }
}
