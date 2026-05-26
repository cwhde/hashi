using Hashi.Infrastructure.Persistence;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Hosting;
using Microsoft.Extensions.Logging;

namespace Hashi.Infrastructure.Platform;

/// <summary>
/// Periodic rollup pass for monitor samples. Check execution remains in <see cref="MonitorCheckWorker"/>.
/// </summary>
public sealed class MonitorRollupWorker(
    IServiceScopeFactory scopeFactory,
    ILogger<MonitorRollupWorker> logger) : BackgroundService
{
    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        while (!stoppingToken.IsCancellationRequested)
        {
            try
            {
                using var scope = scopeFactory.CreateScope();
                var db = scope.ServiceProvider.GetRequiredService<HashiDbContext>();
                var jobs = scope.ServiceProvider.GetRequiredService<BackgroundJobService>();
                await jobs.BeginRunAsync(BackgroundJobKeys.MonitorRollup, stoppingToken);
                var buckets = await MonitorRollupService.RollupRecentAsync(db, stoppingToken);
                await jobs.CompleteRunAsync(
                    BackgroundJobKeys.MonitorRollup,
                    true,
                    buckets > 0 ? $"Updated {buckets} rollup buckets." : "No rollup changes.",
                    null,
                    300,
                    stoppingToken);
                if (buckets > 0)
                {
                    logger.LogDebug("Monitor rollup worker updated {BucketCount} hourly buckets.", buckets);
                }
            }
            catch (Exception ex)
            {
                logger.LogError(ex, "Monitor rollup worker failed.");
                try
                {
                    using var scope = scopeFactory.CreateScope();
                    var jobs = scope.ServiceProvider.GetRequiredService<BackgroundJobService>();
                    await jobs.CompleteRunAsync(BackgroundJobKeys.MonitorRollup, false, null, ex.Message, 300, stoppingToken);
                }
                catch
                {
                }
            }

            await Task.Delay(TimeSpan.FromMinutes(5), stoppingToken);
        }
    }
}
