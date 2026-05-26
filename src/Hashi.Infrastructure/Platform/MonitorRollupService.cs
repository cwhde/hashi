using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Infrastructure.Platform;

public static class MonitorRollupService
{
    public static readonly int[] RollupIntervalsMinutes = [1, 5, 60];

    public static async Task<int> RollupRecentAsync(HashiDbContext db, CancellationToken cancellationToken = default)
    {
        var now = DateTimeOffset.UtcNow;
        var total = 0;
        total += await RollupIntervalAsync(db, intervalMinutes: 1, since: now.AddHours(-3), cancellationToken);
        total += await RollupIntervalAsync(db, intervalMinutes: 5, since: now.AddDays(-1), cancellationToken);
        total += await RollupIntervalAsync(db, intervalMinutes: 60, since: now.AddDays(-7), cancellationToken);
        await PruneOldRollupsAsync(db, cancellationToken);
        return total;
    }

    private static async Task<int> RollupIntervalAsync(
        HashiDbContext db,
        int intervalMinutes,
        DateTimeOffset since,
        CancellationToken cancellationToken)
    {
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
            BucketStart = FloorToBucket(x.CheckedAtUtc, intervalMinutes),
        });

        var count = 0;
        foreach (var group in grouped)
        {
            var existing = await db.MonitorRollups.SingleOrDefaultAsync(
                x => x.MonitorEndpointId == group.Key.MonitorEndpointId
                     && x.BucketStartUtc == group.Key.BucketStart
                     && x.IntervalMinutes == intervalMinutes,
                cancellationToken);
            if (existing is null)
            {
                existing = new MonitorRollupEntity
                {
                    MonitorEndpointId = group.Key.MonitorEndpointId,
                    BucketStartUtc = group.Key.BucketStart,
                    IntervalMinutes = intervalMinutes,
                };
                db.MonitorRollups.Add(existing);
            }

            existing.SampleCount = group.Count();
            existing.UpCount = group.Count(x => string.Equals(x.Status, "up", StringComparison.OrdinalIgnoreCase));
            existing.DownCount = group.Count(x => !string.Equals(x.Status, "up", StringComparison.OrdinalIgnoreCase));
            existing.AverageLatencyMs = group.Average(x => x.LatencyMs);
            count++;
        }

        await db.SaveChangesAsync(cancellationToken);
        return count;
    }

    private static async Task PruneOldRollupsAsync(HashiDbContext db, CancellationToken cancellationToken)
    {
        var now = DateTimeOffset.UtcNow;
        await db.MonitorRollups
            .Where(x => x.IntervalMinutes == 1 && x.BucketStartUtc < now.AddDays(-90))
            .ExecuteDeleteAsync(cancellationToken);
        await db.MonitorRollups
            .Where(x => x.IntervalMinutes == 5 && x.BucketStartUtc < now.AddDays(-180))
            .ExecuteDeleteAsync(cancellationToken);
        await db.MonitorRollups
            .Where(x => x.IntervalMinutes == 60 && x.BucketStartUtc < now.AddDays(-730))
            .ExecuteDeleteAsync(cancellationToken);
    }

    internal static DateTimeOffset FloorToBucket(DateTimeOffset time, int intervalMinutes)
    {
        var utc = time.UtcDateTime;
        var totalMinutes = (long)(utc - DateTime.UnixEpoch).TotalMinutes;
        var flooredMinutes = totalMinutes - (totalMinutes % intervalMinutes);
        return DateTimeOffset.FromUnixTimeSeconds(flooredMinutes * 60);
    }
}
