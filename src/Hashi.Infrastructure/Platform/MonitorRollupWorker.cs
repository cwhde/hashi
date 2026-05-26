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
                var buckets = await MonitorRollupService.RollupRecentAsync(db, stoppingToken);
                if (buckets > 0)
                {
                    logger.LogDebug("Monitor rollup worker updated {BucketCount} hourly buckets.", buckets);
                }
            }
            catch (Exception ex)
            {
                logger.LogError(ex, "Monitor rollup worker failed.");
            }

            await Task.Delay(TimeSpan.FromMinutes(5), stoppingToken);
        }
    }
}
