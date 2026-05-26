using Hashi.Infrastructure.Persistence;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Hosting;
using Microsoft.Extensions.Logging;

namespace Hashi.Infrastructure.Platform;

public sealed class ScriptCronHostedService(
    IServiceScopeFactory scopeFactory,
    ILogger<ScriptCronHostedService> logger) : BackgroundService
{
    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        while (!stoppingToken.IsCancellationRequested)
        {
            try
            {
                using var scope = scopeFactory.CreateScope();
                var db = scope.ServiceProvider.GetRequiredService<HashiDbContext>();
                var dueCount = await db.Scripts.CountAsync(x => x.Enabled && x.CronExpression != "", stoppingToken);
                if (dueCount > 0)
                {
                    logger.LogInformation("Script cron tick: {Count} enabled scripts with schedules.", dueCount);
                }
            }
            catch (Exception ex)
            {
                logger.LogError(ex, "Script cron worker failed.");
            }

            await Task.Delay(TimeSpan.FromMinutes(1), stoppingToken);
        }
    }
}
