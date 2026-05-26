using System.Diagnostics;
using System.Net.Http.Headers;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Hosting;
using Microsoft.Extensions.Logging;

namespace Hashi.Infrastructure.Platform;

public sealed class MonitorCheckWorker(
    IServiceScopeFactory scopeFactory,
    IHttpClientFactory httpClientFactory,
    ILogger<MonitorCheckWorker> logger) : BackgroundService
{
    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        while (!stoppingToken.IsCancellationRequested)
        {
            try
            {
                await RunChecksAsync(stoppingToken);
            }
            catch (Exception ex)
            {
                logger.LogError(ex, "Monitor check worker failed.");
            }

            await Task.Delay(TimeSpan.FromSeconds(30), stoppingToken);
        }
    }

    private async Task RunChecksAsync(CancellationToken cancellationToken)
    {
        using var scope = scopeFactory.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<HashiDbContext>();
        var endpoints = await db.MonitorEndpoints.Where(x => x.Enabled).ToListAsync(cancellationToken);
        var client = httpClientFactory.CreateClient("monitor-checks");
        client.Timeout = TimeSpan.FromSeconds(15);

        foreach (var endpoint in endpoints)
        {
            var sw = Stopwatch.StartNew();
            var status = "down";
            try
            {
                using var response = await client.GetAsync(endpoint.Url, cancellationToken);
                status = response.IsSuccessStatusCode ? "up" : "down";
            }
            catch
            {
                status = "down";
            }

            sw.Stop();
            endpoint.Status = status;
            endpoint.LastCheckedAtUtc = DateTimeOffset.UtcNow;
            endpoint.LastLatencyMs = (int)sw.ElapsedMilliseconds;

            var sample = new MonitorSampleEntity
            {
                MonitorEndpointId = endpoint.Id,
                PartitionDate = DateOnly.FromDateTime(DateTime.UtcNow),
                CheckedAtUtc = DateTimeOffset.UtcNow,
                Status = status,
                LatencyMs = (int)sw.ElapsedMilliseconds,
            };
            db.MonitorSamples.Add(sample);
        }

        await db.SaveChangesAsync(cancellationToken);
        await RollupRecentAsync(db, cancellationToken);
        await PruneOldSamplesAsync(db, cancellationToken);
    }

    private static async Task RollupRecentAsync(HashiDbContext db, CancellationToken cancellationToken)
    {
        var since = DateTimeOffset.UtcNow.AddHours(-1);
        var samples = await db.MonitorSamples.AsNoTracking()
            .Where(x => x.CheckedAtUtc >= since)
            .ToListAsync(cancellationToken);
        var grouped = samples.GroupBy(x => new { x.MonitorEndpointId, Hour = x.CheckedAtUtc.UtcDateTime.Date.AddHours(x.CheckedAtUtc.UtcDateTime.Hour) });
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
    }

    private static async Task PruneOldSamplesAsync(HashiDbContext db, CancellationToken cancellationToken)
    {
        var cutoff = DateOnly.FromDateTime(DateTime.UtcNow.AddDays(-30));
        await db.MonitorSamples.Where(x => x.PartitionDate < cutoff).ExecuteDeleteAsync(cancellationToken);
    }
}
