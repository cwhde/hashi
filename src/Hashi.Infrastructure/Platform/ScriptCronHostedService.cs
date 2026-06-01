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
                var scripts = scope.ServiceProvider.GetRequiredService<ScriptExecutionService>();
                var jobs = scope.ServiceProvider.GetRequiredService<BackgroundJobService>();
                await jobs.BeginRunAsync(BackgroundJobKeys.ScriptCron, stoppingToken);
                await scripts.SyncAllEnabledScriptsAsync(stoppingToken);

                await jobs.CompleteRunAsync(
                    BackgroundJobKeys.ScriptCron,
                    true,
                    "Synced host script files, manifest, and cron entries.",
                    null,
                    60,
                    stoppingToken);
            }
            catch (Exception ex)
            {
                logger.LogError(ex, "Script cron worker failed.");
                try
                {
                    using var scope = scopeFactory.CreateScope();
                    var jobs = scope.ServiceProvider.GetRequiredService<BackgroundJobService>();
                    await jobs.CompleteRunAsync(BackgroundJobKeys.ScriptCron, false, null, ex.Message, 60, stoppingToken);
                }
                catch
                {
                }
            }

            await Task.Delay(TimeSpan.FromMinutes(1), stoppingToken);
        }
    }
}
