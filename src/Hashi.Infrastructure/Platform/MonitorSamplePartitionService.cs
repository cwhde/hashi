using Hashi.Infrastructure.Persistence;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Infrastructure.Platform;

internal static class MonitorSamplePartitionService
{
    public const string ParentTable = "monitor_samples_raw";

    public static DateOnly GetWeekStart(DateOnly date)
    {
        var daysFromMonday = ((int)date.DayOfWeek + 6) % 7;
        return date.AddDays(-daysFromMonday);
    }

    public static string GetPartitionName(DateOnly weekStart)
        => $"{ParentTable}_{weekStart:yyyyMMdd}";

    public static async Task EnsureWeeklyPartitionsAsync(HashiDbContext db, CancellationToken cancellationToken = default)
    {
        var today = DateOnly.FromDateTime(DateTime.UtcNow);
        var currentWeek = GetWeekStart(today);
        await EnsurePartitionAsync(db, currentWeek, cancellationToken);
        await EnsurePartitionAsync(db, currentWeek.AddDays(7), cancellationToken);
    }

    public static async Task EnsurePartitionAsync(
        HashiDbContext db,
        DateOnly weekStart,
        CancellationToken cancellationToken = default)
    {
        var partitionName = GetPartitionName(weekStart);
        var weekEnd = weekStart.AddDays(7);
        var createSql = FormattableString.Invariant(
            $"""
            CREATE TABLE IF NOT EXISTS {QuoteIdentifier(partitionName)}
                PARTITION OF monitor_samples_raw
                FOR VALUES FROM ('{weekStart:yyyy-MM-dd}') TO ('{weekEnd:yyyy-MM-dd}');
            """);
        await db.Database.ExecuteSqlRawAsync(createSql, cancellationToken);
    }

    public static async Task<int> DropExpiredPartitionsAsync(
        HashiDbContext db,
        int retentionDays,
        CancellationToken cancellationToken = default)
    {
        var cutoff = DateOnly.FromDateTime(DateTime.UtcNow.AddDays(-retentionDays));
        var partitionNames = await db.Database.SqlQueryRaw<string>(
                """
                SELECT c.relname::text AS "Value"
                FROM pg_inherits i
                JOIN pg_class c ON c.oid = i.inhrelid
                JOIN pg_class p ON p.oid = i.inhparent
                WHERE p.relname = 'monitor_samples_raw'
                """)
            .ToListAsync(cancellationToken);

        var dropped = 0;
        foreach (var partitionName in partitionNames)
        {
            if (!TryParsePartitionWeekStart(partitionName, out var weekStart))
            {
                continue;
            }

            var weekEnd = weekStart.AddDays(7);
            if (weekEnd > cutoff)
            {
                continue;
            }

            var dropSql = FormattableString.Invariant($"""DROP TABLE IF EXISTS {QuoteIdentifier(partitionName)};""");
            await db.Database.ExecuteSqlRawAsync(dropSql, cancellationToken);
            dropped++;
        }

        return dropped;
    }

    private static string QuoteIdentifier(string identifier)
        => "\"" + identifier.Replace("\"", "\"\"", StringComparison.Ordinal) + "\"";

    internal static bool TryParsePartitionWeekStart(string partitionName, out DateOnly weekStart)
    {
        weekStart = default;
        const string prefix = ParentTable + "_";
        if (!partitionName.StartsWith(prefix, StringComparison.Ordinal))
        {
            return false;
        }

        var suffix = partitionName[prefix.Length..];
        return DateOnly.TryParseExact(suffix, "yyyyMMdd", out weekStart);
    }
}
