using Hashi.Contracts.Api;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Sync;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Infrastructure.Services;

public sealed class SystemResourceSetupService(
    HashiDbContext db,
    AppSettingsService settings,
    SyncOrchestratorService sync)
{
    public async Task<SystemResourceSyncResponse> PlanAsync(CancellationToken cancellationToken = default)
    {
        await EnsureSystemResourceAsync(cancellationToken);
        var plan = await sync.PlanGlobalAsync(cancellationToken);
        return new SystemResourceSyncResponse(
            true,
            plan.PlanId,
            plan.RiskLevel,
            plan.RequiresConfirmation,
            plan.PreviewMarkdown,
            "Plan ready.");
    }

    public async Task<SystemResourceSyncResponse> SyncAsync(CancellationToken cancellationToken = default)
    {
        await EnsureSystemResourceAsync(cancellationToken);
        var apply = await sync.ApplyGlobalAsync(confirmDestructive: false, cancellationToken);
        return new SystemResourceSyncResponse(
            apply.Succeeded,
            apply.RunId,
            null,
            false,
            null,
            apply.Message ?? "System resource sync applied.");
    }

    private async Task EnsureSystemResourceAsync(CancellationToken cancellationToken)
    {
        var appSettings = await settings.GetOrCreateAsync(cancellationToken);
        if (string.IsNullOrWhiteSpace(appSettings.AdminDomain))
        {
            throw new InvalidOperationException("Admin domain must be configured before system resource sync.");
        }

        var existing = await db.Resources.SingleOrDefaultAsync(x => x.IsSystem, cancellationToken);
        if (existing is not null)
        {
            existing.Domain = appSettings.AdminDomain;
            existing.Enabled = true;
            await db.SaveChangesAsync(cancellationToken);
            return;
        }

        db.Resources.Add(new ResourceEntity
        {
            Name = "Hashi Admin",
            Slug = "hashi-admin",
            Kind = "https",
            Domain = appSettings.AdminDomain,
            TargetScheme = "http",
            TargetHost = "127.0.0.1",
            TargetPort = 8080,
            Enabled = true,
            IsSystem = true,
            DashboardEnabled = false,
            StatusEnabled = true,
        });
        await db.SaveChangesAsync(cancellationToken);
    }
}
