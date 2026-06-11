namespace Hashi.Api.Features.Platform;

using Hashi.Contracts.Api;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Platform;
using Hashi.Infrastructure.Services;
using Hashi.Infrastructure.Sync;
using Hashi.Infrastructure.Auth;
using Microsoft.AspNetCore.Builder;
using Microsoft.AspNetCore.Http;
using Microsoft.AspNetCore.Routing;
using Microsoft.EntityFrameworkCore;
using System.Linq;
using System.Threading;
using System.Threading.Tasks;
using System.Collections.Generic;
using Microsoft.AspNetCore.Http.HttpResults;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Dns;

public static class DashboardEndpoints
{
    public static IEndpointRouteBuilder MapDashboardEndpoints(this IEndpointRouteBuilder app)
    {
        app.MapGet("/api/dashboard", async (
            AuditService auditService,
            VaultSessionState vaultSession,
            ServiceSyncVaultState serviceSync,
            VaultService vaultService,
            ResourceService resourceService,
            MonitoringService monitoringService,
            SecurityIngestionService securityService,
            HashiDbContext db,
            SyncRunService syncRunService,
            CancellationToken ct) =>
        {
            // 1. Audit events (recent 5 events, as in svelte slice(0,5))
            var events = await auditService.ListRecentAsync(cancellationToken: ct);
            var auditEventsResponse = events.Select(x => new AuditEventResponse(
                x.Id,
                x.Category,
                x.Action,
                x.SubjectType,
                x.SubjectId,
                x.Outcome,
                x.CreatedAtUtc)).ToList();

            // 2. Health
            var available = vaultSession.IsUnlocked || (serviceSync.IsReady && serviceSync.IsUnlocked);
            var healthResponse = new HealthResponse(
                "healthy",
                "2.0.0-alpha",
                DateTimeOffset.UtcNow,
                serviceSync.IsReady,
                ProviderSyncPaused: !available);

            // 3. Vault status
            var vaultStatus = await vaultService.GetStatusAsync(ct);
            var vaultResponse = new VaultStatusResponse(
                vaultStatus.LockState.ToString(),
                vaultStatus.IsVaultConfigured,
                vaultStatus.HasPasskey,
                vaultStatus.PrfWrapAvailable,
                vaultStatus.ServiceSyncVaultReady,
                vaultStatus.BootstrapCredentialsActive);

            // 4. Resources
            var resources = await resourceService.ListAsync(ct);
            var resourceResponses = new List<ResourceResponse>();
            foreach (var item in resources)
            {
                resourceResponses.Add(await resourceService.ToResponseAsync(item, ct));
            }

            // 5. Monitors
            var monitorsResponse = await monitoringService.ListResponsesAsync(ct);

            // 6. Security Dashboard
            var securityResponse = await securityService.GetDashboardAsync(
                24,
                resourceFilter: null,
                traefikHostFilter: null,
                firewallHostIdFilter: null,
                cancellationToken: ct);

            // 7. DNS Connections
            var dnsConnectionsList = await db.Connections
                .Where(x => x.Type == ConnectionTypeNames.DnsProvider)
                .AsNoTracking()
                .OrderBy(x => x.Name)
                .ToListAsync(ct);
            var dnsConnectionsResponse = dnsConnectionsList.Select(connection => new ConnectionSummaryResponse(
                connection.Id,
                connection.Name,
                connection.Type,
                connection.Enabled,
                connection.HealthState,
                connection.LastValidationMessage,
                connection.LastValidatedAtUtc)).ToList();

            // 8. Pulse agents
            var pulseAgentsList = await db.PulseAgents.AsNoTracking().ToListAsync(ct);
            var pulseAgentsResponse = pulseAgentsList.Select(PulseAgentService.ToResponse).ToList();

            // 9. Sync runs
            var syncRunsResponse = await syncRunService.ListRecentAsync(50, ct);

            return TypedResults.Ok(new AdminDashboardResponse(
                auditEventsResponse,
                healthResponse,
                vaultResponse,
                resourceResponses,
                monitorsResponse,
                securityResponse,
                dnsConnectionsResponse,
                pulseAgentsResponse,
                syncRunsResponse));
        })
        .WithName("GetAdminDashboard")
        .WithTags("Platform");

        return app;
    }
}
