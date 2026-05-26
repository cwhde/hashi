using System.Diagnostics;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Services;
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
            var delaySeconds = 30;
            try
            {
                using var scope = scopeFactory.CreateScope();
                var settings = scope.ServiceProvider.GetRequiredService<AppSettingsService>();
                var appSettings = await settings.GetOrCreateAsync(stoppingToken);
                delaySeconds = Math.Clamp(appSettings.MonitorCheckIntervalSeconds, 15, 300);

                var jobs = scope.ServiceProvider.GetRequiredService<BackgroundJobService>();
                await jobs.BeginRunAsync(BackgroundJobKeys.MonitorCheck, stoppingToken);
                await RunChecksAsync(appSettings.MonitorCheckTimeoutSeconds, appSettings.MonitorDegradedLatencyMs, stoppingToken);
                await jobs.CompleteRunAsync(
                    BackgroundJobKeys.MonitorCheck,
                    true,
                    "Monitor checks completed.",
                    null,
                    delaySeconds,
                    stoppingToken);
            }
            catch (Exception ex)
            {
                logger.LogError(ex, "Monitor check worker failed.");
                try
                {
                    using var scope = scopeFactory.CreateScope();
                    var jobs = scope.ServiceProvider.GetRequiredService<BackgroundJobService>();
                    await jobs.CompleteRunAsync(BackgroundJobKeys.MonitorCheck, false, null, ex.Message, delaySeconds, stoppingToken);
                }
                catch
                {
                }
            }

            await Task.Delay(TimeSpan.FromSeconds(delaySeconds), stoppingToken);
        }
    }

    private async Task RunChecksAsync(int timeoutSeconds, int degradedLatencyMs, CancellationToken cancellationToken)
    {
        using var scope = scopeFactory.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<HashiDbContext>();
        var monitoring = scope.ServiceProvider.GetRequiredService<MonitoringService>();
        var appSettings = scope.ServiceProvider.GetRequiredService<AppSettingsService>();
        await monitoring.SyncEndpointsFromResourcesAsync(cancellationToken);
        var endpoints = await db.MonitorEndpoints.Where(x => x.Enabled).ToListAsync(cancellationToken);
        var client = httpClientFactory.CreateClient("monitor-checks");
        client.Timeout = TimeSpan.FromSeconds(Math.Clamp(timeoutSeconds, 5, 120));
        var retentionDays = Math.Clamp((await appSettings.GetOrCreateAsync(cancellationToken)).MonitorSampleRetentionDays, 7, 365);

        foreach (var endpoint in endpoints)
        {
            var previousStatus = endpoint.Status;
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
            var latency = (int)sw.ElapsedMilliseconds;
            if (status == "up" && latency >= degradedLatencyMs)
            {
                status = "degraded";
            }

            endpoint.Status = status;
            endpoint.LastCheckedAtUtc = DateTimeOffset.UtcNow;
            endpoint.LastLatencyMs = latency;

            var sample = new MonitorSampleEntity
            {
                MonitorEndpointId = endpoint.Id,
                PartitionDate = DateOnly.FromDateTime(DateTime.UtcNow),
                CheckedAtUtc = DateTimeOffset.UtcNow,
                Status = status,
                LatencyMs = latency,
            };
            db.MonitorSamples.Add(sample);

            await monitoring.RecordTransitionAsync(endpoint, previousStatus, status, latency, cancellationToken);

            var routing = scope.ServiceProvider.GetRequiredService<NotificationRoutingService>();
            await routing.RouteMonitorTransitionAsync(endpoint, previousStatus, status, cancellationToken);
        }

        await db.SaveChangesAsync(cancellationToken);
        await MonitorRollupService.RollupRecentAsync(db, cancellationToken);
        await PruneOldSamplesAsync(db, retentionDays, cancellationToken);
    }

    private static async Task PruneOldSamplesAsync(HashiDbContext db, int retentionDays, CancellationToken cancellationToken)
    {
        var cutoff = DateOnly.FromDateTime(DateTime.UtcNow.AddDays(-retentionDays));
        await db.MonitorSamples.Where(x => x.PartitionDate < cutoff).ExecuteDeleteAsync(cancellationToken);
    }
}
