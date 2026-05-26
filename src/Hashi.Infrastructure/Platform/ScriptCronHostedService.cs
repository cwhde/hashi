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
                var scripts = scope.ServiceProvider.GetRequiredService<ScriptExecutionService>();
                var jobs = scope.ServiceProvider.GetRequiredService<BackgroundJobService>();
                await jobs.BeginRunAsync(BackgroundJobKeys.ScriptCron, stoppingToken);
                await scripts.SyncAllEnabledScriptsAsync(stoppingToken);
                var now = DateTimeOffset.UtcNow;
                var dueScripts = await db.Scripts
                    .Where(x => x.Enabled && x.CronExpression != "")
                    .ToListAsync(stoppingToken);
                var ran = 0;
                foreach (var script in dueScripts)
                {
                    if (!ScriptCronSchedule.IsDue(script.CronExpression, script.LastRunAtUtc, now))
                    {
                        continue;
                    }

                    try
                    {
                        var result = await scripts.RunWithConnectionAsync(script.Id, stoppingToken);
                        ran++;
                        if (result.Succeeded)
                        {
                            logger.LogInformation("Script cron run succeeded for {ScriptName} ({ScriptId}).", script.Name, script.Id);
                        }
                        else
                        {
                            logger.LogWarning(
                                "Script cron run failed for {ScriptName} ({ScriptId}): {Error}",
                                script.Name,
                                script.Id,
                                result.Error ?? "unknown error");
                        }
                    }
                    catch (Exception ex)
                    {
                        logger.LogError(ex, "Script cron run threw for {ScriptName} ({ScriptId}).", script.Name, script.Id);
                    }
                }

                await jobs.CompleteRunAsync(
                    BackgroundJobKeys.ScriptCron,
                    true,
                    $"Synced scripts; executed {ran} due cron run(s).",
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
