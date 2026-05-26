using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Infrastructure.Platform;

public static class MonitorRollupService
{
    public static async Task<int> RollupRecentAsync(HashiDbContext db, CancellationToken cancellationToken = default)
    {
        var since = DateTimeOffset.UtcNow.AddHours(-1);
        var samples = await db.MonitorSamples.AsNoTracking()
            .Where(x => x.CheckedAtUtc >= since)
            .ToListAsync(cancellationToken);
        if (samples.Count == 0)
        {
            return 0;
        }

        var grouped = samples.GroupBy(x => new
        {
            x.MonitorEndpointId,
            Hour = x.CheckedAtUtc.UtcDateTime.Date.AddHours(x.CheckedAtUtc.UtcDateTime.Hour),
        });
        foreach (var group in grouped)
        {
            var bucketStart = new DateTimeOffset(group.Key.Hour, TimeSpan.Zero);
            var existing = await db.MonitorRollups.SingleOrDefaultAsync(
                x => x.MonitorEndpointId == group.Key.MonitorEndpointId && x.BucketStartUtc == bucketStart,
                cancellationToken);
            if (existing is null)
            {
                existing = new MonitorRollupEntity
                {
                    MonitorEndpointId = group.Key.MonitorEndpointId,
                    BucketStartUtc = bucketStart,
                };
                db.MonitorRollups.Add(existing);
            }

            existing.SampleCount = group.Count();
            existing.UpCount = group.Count(x => x.Status == "up");
            existing.DownCount = group.Count(x => x.Status != "up");
            existing.AverageLatencyMs = group.Average(x => x.LatencyMs);
        }

        await db.SaveChangesAsync(cancellationToken);
        return grouped.Count();
    }
}
