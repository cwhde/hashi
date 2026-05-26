using Hashi.Contracts.Api;
using Hashi.Core.Sync;
using Hashi.Infrastructure.Dns;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Platform;
using Hashi.Infrastructure.Services;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Hosting;
using Microsoft.Extensions.Logging;

namespace Hashi.Infrastructure.Sync;

public sealed class SyncRunService(HashiDbContext db)
{
    public async Task<SyncRunEntity> BeginRunAsync(string subsystem, CancellationToken cancellationToken = default)
    {
        var run = new SyncRunEntity
        {
            Subsystem = subsystem,
            Status = SyncRunStatusNames.Planning,
        };
        db.SyncRuns.Add(run);
        await db.SaveChangesAsync(cancellationToken);
        return run;
    }

    public async Task AddStepAsync(
        Guid runId,
        string name,
        string status,
        string? message,
        CancellationToken cancellationToken = default)
    {
        db.SyncSteps.Add(new SyncStepEntity
        {
            SyncRunId = runId,
            Name = name,
            Status = status,
            Message = message,
            CompletedAtUtc = status is SyncRunStatusNames.Succeeded or SyncRunStatusNames.Failed
                ? DateTimeOffset.UtcNow
                : null,
        });
        await db.SaveChangesAsync(cancellationToken);
    }

    public async Task AddDiffsAsync(
        Guid runId,
        IEnumerable<ProviderChange> changes,
        CancellationToken cancellationToken = default)
    {
        foreach (var change in changes)
        {
            db.SyncDiffs.Add(new SyncDiffEntity
            {
                SyncRunId = runId,
                ResourceType = change.ResourceType,
                ResourceKey = change.ResourceKey,
                ChangeKind = change.Kind.ToString(),
                Summary = change.Summary,
            });
        }

        await db.SaveChangesAsync(cancellationToken);
    }

    public async Task CompleteRunAsync(
        Guid runId,
        string status,
        SyncRiskLevel riskLevel,
        string? errorSummary,
        CancellationToken cancellationToken = default)
    {
        var run = await db.SyncRuns.SingleAsync(x => x.Id == runId, cancellationToken);
        run.Status = status;
        run.RiskLevel = riskLevel.ToString();
        run.ErrorSummary = errorSummary;
        run.CompletedAtUtc = DateTimeOffset.UtcNow;
        await db.SaveChangesAsync(cancellationToken);
    }

    public async Task<IReadOnlyList<SyncRunResponse>> ListRecentAsync(int limit, CancellationToken cancellationToken = default)
    {
        var runs = await db.SyncRuns.AsNoTracking()
            .OrderByDescending(x => x.StartedAtUtc)
            .Take(limit)
            .Include(x => x.Steps)
            .Include(x => x.Diffs)
            .ToListAsync(cancellationToken);

        return runs.Select(ToResponse).ToList();
    }

    public async Task<SyncRunResponse?> GetAsync(Guid id, CancellationToken cancellationToken = default)
    {
        var run = await db.SyncRuns.AsNoTracking()
            .Include(x => x.Steps)
            .Include(x => x.Diffs)
            .SingleOrDefaultAsync(x => x.Id == id, cancellationToken);
        return run is null ? null : ToResponse(run);
    }

    private static SyncRunResponse ToResponse(SyncRunEntity run) => new(
        run.Id,
        run.Subsystem,
        run.Status,
        run.RiskLevel,
        run.StartedAtUtc,
        run.CompletedAtUtc,
        run.ErrorSummary,
        run.Steps.Select(s => new SyncStepResponse(s.Id, s.Name, s.Status, s.StartedAtUtc, s.CompletedAtUtc, s.Message)).ToList(),
        run.Diffs.Select(d => new SyncDiffResponse(d.Id, d.ResourceType, d.ResourceKey, d.ChangeKind, d.Summary)).ToList());
}

public sealed class SyncOrchestratorService(
    HashiDbContext db,
    DnsConnectionService dns,
    TraefikPlatformService traefik,
    FirewallApplyService firewall,
    AdGuardSyncService adguard,
    SyncRunService syncRuns,
    AppSettingsService settings,
    AuditService audit)
{
    public async Task<SyncPlanPreviewResponse> PlanGlobalAsync(CancellationToken cancellationToken = default)
    {
        var run = await syncRuns.BeginRunAsync("global", cancellationToken);
        var changes = new List<ProviderChange>();
        var risk = SyncRiskLevel.None;

        await syncRuns.AddStepAsync(run.Id, "dns-plan", SyncRunStatusNames.Planning, null, cancellationToken);
        var dnsConnections = await db.Connections.AsNoTracking()
            .Where(x => x.Type == ConnectionTypeNames.DnsProvider && x.Enabled)
            .ToListAsync(cancellationToken);
        foreach (var connection in dnsConnections)
        {
            try
            {
                var plan = await dns.PlanSyncAsync(connection.Id, cancellationToken);
                changes.AddRange(plan.Changes.Select(c => new ProviderChange(
                    "dns",
                    $"{c.Name}/{c.Type}",
                    MapDnsKind(c.Kind),
                    c.RiskReason)));
                if (plan.RequiresConfirmation)
                {
                    risk = MaxRisk(risk, SyncRiskLevel.Destructive);
                }
                else if (plan.Changes.Any(x => x.Kind != Core.Dns.DnsChangeKind.NoOp))
                {
                    risk = MaxRisk(risk, SyncRiskLevel.Low);
                }
            }
            catch (Exception ex)
            {
                await syncRuns.AddStepAsync(run.Id, $"dns-plan-{connection.Id}", SyncRunStatusNames.Failed, ex.Message, cancellationToken);
            }
        }

        await syncRuns.AddStepAsync(run.Id, "traefik-plan", SyncRunStatusNames.Planning, null, cancellationToken);
        var render = await traefik.RenderAsync(cancellationToken);
        changes.Add(new ProviderChange("traefik", "dynamic-config", ProviderResultKind.Updated, $"Hash {render.ContentHash}"));

        await syncRuns.AddDiffsAsync(run.Id, changes, cancellationToken);
        var requiresConfirmation = risk >= SyncRiskLevel.High;
        await syncRuns.CompleteRunAsync(
            run.Id,
            requiresConfirmation ? SyncRunStatusNames.AwaitingConfirmation : SyncRunStatusNames.Succeeded,
            risk,
            null,
            cancellationToken);

        return new SyncPlanPreviewResponse(
            run.Id,
            "global",
            risk.ToString(),
            requiresConfirmation,
            changes.Select(c => new SyncDiffResponse(Guid.Empty, c.ResourceType, c.ResourceKey, c.Kind.ToString(), c.Summary)).ToList(),
            BuildPreviewMarkdown(changes));
    }

    public async Task<SyncApplyResponse> ApplyGlobalAsync(bool confirmDestructive, CancellationToken cancellationToken = default)
    {
        var run = await syncRuns.BeginRunAsync("global-apply", cancellationToken);
        try
        {
            var dnsConnections = await db.Connections
                .Where(x => x.Type == ConnectionTypeNames.DnsProvider && x.Enabled)
                .ToListAsync(cancellationToken);
            foreach (var connection in dnsConnections)
            {
                await syncRuns.AddStepAsync(run.Id, $"dns-apply-{connection.Name}", SyncRunStatusNames.Applying, null, cancellationToken);
                var plan = await dns.PlanSyncAsync(connection.Id, cancellationToken);
                if (plan.RequiresConfirmation && !confirmDestructive)
                {
                    await syncRuns.CompleteRunAsync(run.Id, SyncRunStatusNames.AwaitingConfirmation, SyncRiskLevel.Destructive, "Destructive DNS changes require confirmation.", cancellationToken);
                    return new SyncApplyResponse(run.Id, false, SyncRunStatusNames.AwaitingConfirmation, "Destructive DNS changes require confirmation.");
                }

                await dns.ApplyPlanAsync(plan, confirmDestructive, cancellationToken);
                await syncRuns.AddStepAsync(run.Id, $"dns-apply-{connection.Name}", SyncRunStatusNames.Succeeded, "Applied", cancellationToken);
            }

            await syncRuns.AddStepAsync(run.Id, "adguard-sync", SyncRunStatusNames.Applying, null, cancellationToken);
            var adguardConnections = await db.AdGuardConnections.ToListAsync(cancellationToken);
            foreach (var connection in adguardConnections)
            {
                await adguard.SyncManagedRewritesAsync(connection.Id, cancellationToken);
            }

            await syncRuns.AddStepAsync(run.Id, "adguard-sync", SyncRunStatusNames.Succeeded, "Applied", cancellationToken);

            await syncRuns.CompleteRunAsync(run.Id, SyncRunStatusNames.Succeeded, SyncRiskLevel.Low, null, cancellationToken);
            await audit.WriteAsync("sync", "global_apply", subjectType: "sync_run", subjectId: run.Id.ToString(), cancellationToken: cancellationToken);
            return new SyncApplyResponse(run.Id, true, SyncRunStatusNames.Succeeded, null);
        }
        catch (Exception ex)
        {
            await syncRuns.CompleteRunAsync(run.Id, SyncRunStatusNames.Failed, SyncRiskLevel.High, ex.Message, cancellationToken);
            return new SyncApplyResponse(run.Id, false, SyncRunStatusNames.Failed, ex.Message);
        }
    }

    public async Task<SyncReconcileResponse> ReconcileAsync(CancellationToken cancellationToken = default)
    {
        _ = await settings.GetOrCreateAsync(cancellationToken);
        var run = await syncRuns.BeginRunAsync("reconcile", cancellationToken);
        var subsystems = new List<string>();
        try
        {
            subsystems.Add("dns");
            await syncRuns.AddStepAsync(run.Id, "dns-reconcile", SyncRunStatusNames.Reconciling, null, cancellationToken);
            var dnsConnections = await db.Connections.Where(x => x.Type == ConnectionTypeNames.DnsProvider && x.Enabled).ToListAsync(cancellationToken);
            foreach (var connection in dnsConnections)
            {
                await dns.PlanSyncAsync(connection.Id, cancellationToken);
            }

            subsystems.Add("traefik");
            await traefik.RenderAsync(cancellationToken);

            subsystems.Add("adguard");
            var adguardConnections = await db.AdGuardConnections.ToListAsync(cancellationToken);
            foreach (var connection in adguardConnections)
            {
                await adguard.SyncManagedRewritesAsync(connection.Id, cancellationToken);
            }

            if (await db.FirewallHosts.AnyAsync(cancellationToken))
            {
                subsystems.Add("firewall");
                _ = firewall;
            }

            await syncRuns.CompleteRunAsync(run.Id, SyncRunStatusNames.Succeeded, SyncRiskLevel.None, null, cancellationToken);
            return new SyncReconcileResponse(run.Id, true, subsystems);
        }
        catch (Exception ex)
        {
            await syncRuns.CompleteRunAsync(run.Id, SyncRunStatusNames.Failed, SyncRiskLevel.Medium, ex.Message, cancellationToken);
            return new SyncReconcileResponse(run.Id, false, subsystems);
        }
    }

    private static SyncRiskLevel MaxRisk(SyncRiskLevel current, SyncRiskLevel next)
        => (SyncRiskLevel)Math.Max((int)current, (int)next);

    private static ProviderResultKind MapDnsKind(Core.Dns.DnsChangeKind kind) => kind switch
    {
        Core.Dns.DnsChangeKind.Create => ProviderResultKind.Created,
        Core.Dns.DnsChangeKind.Update => ProviderResultKind.Updated,
        Core.Dns.DnsChangeKind.Delete => ProviderResultKind.Deleted,
        _ => ProviderResultKind.NoOp,
    };

    private static string? BuildPreviewMarkdown(IReadOnlyList<ProviderChange> changes)
    {
        if (changes.Count == 0)
        {
            return "No changes planned.";
        }

        return string.Join('\n', changes.Select(c => $"- **{c.ResourceType}** `{c.ResourceKey}`: {c.Kind} — {c.Summary}"));
    }
}

public sealed class SyncOrchestratorHostedService(
    IServiceScopeFactory scopeFactory,
    ILogger<SyncOrchestratorHostedService> logger) : BackgroundService
{
    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        await Task.Delay(TimeSpan.FromMinutes(1), stoppingToken);
        while (!stoppingToken.IsCancellationRequested)
        {
            try
            {
                using var scope = scopeFactory.CreateScope();
                var settings = scope.ServiceProvider.GetRequiredService<AppSettingsService>();
                var orchestrator = scope.ServiceProvider.GetRequiredService<SyncOrchestratorService>();
                var general = await settings.GetOrCreateAsync(stoppingToken);
                var interval = Math.Max(5, general.DefaultSyncIntervalMinutes);

                logger.LogInformation("Starting passive sync reconcile.");
                await orchestrator.ReconcileAsync(stoppingToken);
                logger.LogInformation("Passive sync reconcile completed.");
                await Task.Delay(TimeSpan.FromMinutes(interval), stoppingToken);
            }
            catch (OperationCanceledException) when (stoppingToken.IsCancellationRequested)
            {
                break;
            }
            catch (Exception ex)
            {
                logger.LogError(ex, "Passive sync orchestrator failed.");
                await Task.Delay(TimeSpan.FromMinutes(5), stoppingToken);
            }
        }
    }
}
