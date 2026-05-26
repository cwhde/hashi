using Hashi.Contracts.Api;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Infrastructure.Platform;

public sealed class MonitoringService(HashiDbContext db)
{
    public async Task SyncEndpointsFromResourcesAsync(CancellationToken cancellationToken = default)
    {
        var resources = await db.Resources.AsNoTracking()
            .Where(x => x.Enabled && x.StatusEnabled && x.Domain != null && x.Domain != "")
            .ToListAsync(cancellationToken);
        var existing = await db.MonitorEndpoints.ToListAsync(cancellationToken);

        foreach (var resource in resources)
        {
            var checkType = resource.Kind.Equals("http", StringComparison.OrdinalIgnoreCase) ? "http" : "https";
            var url = $"{resource.TargetScheme}://{resource.Domain}/";
            var monitor = existing.SingleOrDefault(x => x.ResourceId == resource.Id)
                ?? existing.SingleOrDefault(x => x.Name == resource.Name);
            if (monitor is null)
            {
                monitor = new MonitorEndpointEntity { ResourceId = resource.Id };
                db.MonitorEndpoints.Add(monitor);
                existing.Add(monitor);
            }

            monitor.ResourceId = resource.Id;
            monitor.Name = resource.Name;
            monitor.Url = url;
            monitor.CheckType = checkType;
            monitor.Enabled = true;
        }

        foreach (var monitor in existing.Where(x => x.ResourceId is not null))
        {
            if (resources.All(r => r.Id != monitor.ResourceId))
            {
                monitor.Enabled = false;
            }
        }

        await db.SaveChangesAsync(cancellationToken);
    }

    public async Task<IReadOnlyList<MonitorEndpointEntity>> ListAsync(CancellationToken cancellationToken = default)
        => await db.MonitorEndpoints.AsNoTracking().OrderBy(x => x.Name).ToListAsync(cancellationToken);

    public async Task<IReadOnlyList<MonitorRollupEntity>> ListRollupsAsync(
        Guid? endpointId,
        int? intervalMinutes,
        int hours,
        CancellationToken cancellationToken = default)
    {
        var since = DateTimeOffset.UtcNow.AddHours(-Math.Clamp(hours, 1, 720));
        var query = db.MonitorRollups.AsNoTracking().Where(x => x.BucketStartUtc >= since);
        if (endpointId is Guid id)
        {
            query = query.Where(x => x.MonitorEndpointId == id);
        }

        if (intervalMinutes is int interval && interval > 0)
        {
            query = query.Where(x => x.IntervalMinutes == interval);
        }

        return await query.OrderBy(x => x.BucketStartUtc).ToListAsync(cancellationToken);
    }

    public async Task<IReadOnlyList<PublicStatusItemResponse>> PublicStatusAsync(CancellationToken cancellationToken = default)
    {
        var endpoints = await db.MonitorEndpoints.AsNoTracking()
            .Where(x => x.Enabled)
            .OrderBy(x => x.Name)
            .ToListAsync(cancellationToken);
        var since = DateTimeOffset.UtcNow.AddHours(-1);
        var rollups = await db.MonitorRollups.AsNoTracking()
            .Where(x => x.IntervalMinutes == 1 && x.BucketStartUtc >= since)
            .OrderBy(x => x.BucketStartUtc)
            .ToListAsync(cancellationToken);

        return endpoints.Select(endpoint =>
        {
            var strip = rollups
                .Where(x => x.MonitorEndpointId == endpoint.Id)
                .Select(x => new PublicStatusStripBucket(
                    x.BucketStartUtc,
                    x.UpCount >= x.DownCount))
                .ToList();
            return new PublicStatusItemResponse(
                endpoint.Name,
                NormalizeStatus(endpoint.Status),
                endpoint.LastLatencyMs,
                strip);
        }).ToList();
    }

    public static MonitorEndpointResponse ToResponse(MonitorEndpointEntity entity) => new(
        entity.Id,
        entity.Name,
        entity.Url,
        entity.CheckType,
        entity.Enabled,
        NormalizeStatus(entity.Status),
        entity.LastCheckedAtUtc,
        entity.LastLatencyMs);

    public static MonitorRollupResponse ToRollupResponse(MonitorRollupEntity entity) => new(
        entity.MonitorEndpointId,
        entity.BucketStartUtc,
        entity.IntervalMinutes,
        entity.SampleCount,
        entity.UpCount,
        entity.DownCount,
        entity.AverageLatencyMs);

    private static string NormalizeStatus(string status) => status.ToLowerInvariant() switch
    {
        "up" => "Up",
        "down" => "Down",
        "degraded" => "Degraded",
        "paused" => "Paused",
        _ => "Unknown",
    };
}
