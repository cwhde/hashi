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
    public const string HashiAdminSystemKey = "hashi_admin";

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
        var plan = await sync.PlanGlobalAsync(cancellationToken);
        var apply = await sync.ApplyGlobalAsync(plan.PlanId, confirmDestructive: false, cancellationToken);
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

        var system = await db.SystemResources
            .Include(x => x.Resource)
            .SingleOrDefaultAsync(x => x.SystemKey == HashiAdminSystemKey, cancellationToken);
        var existing = system?.Resource;
        if (existing is null)
        {
            existing = await db.Resources
                .Where(x => x.IsSystem)
                .Where(x => x.OwningWorkflow == null || x.OwningWorkflow == "setup")
                .OrderBy(x => x.CreatedAtUtc)
                .FirstOrDefaultAsync(cancellationToken);
        }

        if (existing is not null)
        {
            existing.Domain = appSettings.AdminDomain;
            existing.Enabled = true;
            existing.IsSystem = true;
            existing.Ownership = ResourceOwnershipNames.System;
            existing.OwningWorkflow = "setup";
            existing.DeletionPolicy = ResourceDeletionPolicyNames.RequiredForAccess;
            if (system is null)
            {
                db.SystemResources.Add(new SystemResourceEntity
                {
                    ResourceId = existing.Id,
                    SystemKey = HashiAdminSystemKey,
                    OwningWorkflow = "setup",
                    RequiredForAppAccess = true,
                });
            }

            await db.SaveChangesAsync(cancellationToken);
            return;
        }

        var resource = new ResourceEntity
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
            Ownership = ResourceOwnershipNames.System,
            OwningWorkflow = "setup",
            DeletionPolicy = ResourceDeletionPolicyNames.RequiredForAccess,
            DashboardEnabled = false,
            StatusEnabled = true,
        };
        db.Resources.Add(resource);
        await db.SaveChangesAsync(cancellationToken);
        db.SystemResources.Add(new SystemResourceEntity
        {
            ResourceId = resource.Id,
            SystemKey = HashiAdminSystemKey,
            OwningWorkflow = "setup",
            RequiredForAppAccess = true,
        });
        await db.SaveChangesAsync(cancellationToken);
    }
}
