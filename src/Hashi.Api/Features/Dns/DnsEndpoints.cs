using Hashi.Contracts.Api;
using Hashi.Core.Dns;
using Hashi.Infrastructure.Dns;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Sync;
using Microsoft.AspNetCore.Http.HttpResults;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Api.Features.Dns;

public static class DnsEndpoints
{
    public static IEndpointRouteBuilder MapDnsEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app.MapGroup("/api/dns").WithTags("DNS");

        group.MapPost("/providers/hetzner/validate", async Task<IResult> (
            DnsProviderValidationRequest request,
            IHttpClientFactory httpClientFactory,
            CancellationToken ct) =>
        {
            var (valid, error) = await DnsProviderValidation.ValidateHetznerTokenAsync(httpClientFactory, request.ApiToken, ct);
            return TypedResults.Ok(new DnsProviderValidationResponse(valid, error));
        });

        group.MapGet("/connections", async (HashiDbContext db, CancellationToken ct) =>
        {
            var items = await db.Connections
                .Where(x => x.Type == ConnectionTypeNames.DnsProvider)
                .AsNoTracking()
                .OrderBy(x => x.Name)
                .ToListAsync(ct);
            return TypedResults.Ok(items.Select(ToSummary));
        });

        group.MapPost("/connections/hetzner", async Task<IResult> (
            CreateHetznerDnsConnectionRequest request,
            DnsConnectionService dns,
            SyncOrchestratorService sync,
            CancellationToken ct) =>
        {
            var connection = await dns.CreateHetznerConnectionAsync(
                request.Name,
                request.ApiToken,
                request.ZoneName,
                request.DefaultTtl,
                ct);
            await sync.TriggerImmediateSyncAsync(ct);
            return TypedResults.Ok(ToSummary(connection));
        });

        group.MapPost("/connections/{connectionId:guid}/validate", async Task<IResult> (
            Guid connectionId,
            DnsConnectionService dns,
            CancellationToken ct) =>
        {
            var (valid, error) = await dns.ValidateConnectionAsync(connectionId, ct);
            return TypedResults.Ok(new DnsProviderValidationResponse(valid, error));
        });

        group.MapPost("/connections/{connectionId:guid}/validate-write", async Task<IResult> (
            Guid connectionId,
            DnsWriteValidationRequest request,
            DnsConnectionService dns,
            CancellationToken ct) =>
        {
            var (valid, error) = await dns.ValidateWriteAsync(connectionId, request.ConfirmDryRun, ct);
            return TypedResults.Ok(new DnsWriteValidationResponse(valid, error));
        });

        group.MapGet("/connections/{connectionId:guid}/capabilities", async Task<IResult> (
            Guid connectionId,
            DnsConnectionService dns,
            CancellationToken ct) =>
        {
            var caps = await dns.GetCapabilitiesAsync(connectionId, ct);
            return TypedResults.Ok(new DnsProviderCapabilitiesResponse(
                caps.SupportedRecordTypes,
                caps.SupportsBatchOperations,
                caps.MaxRecordsPerZone,
                caps.SupportsComments,
                caps.RateLimitLimit,
                caps.RateLimitWindowSeconds));
        })
            .Produces<DnsProviderCapabilitiesResponse>(StatusCodes.Status200OK);

        group.MapGet("/connections/{connectionId:guid}/records/provider", async Task<IResult> (
            Guid connectionId,
            DnsConnectionService dns,
            CancellationToken ct) =>
        {
            var records = await dns.ListProviderRecordsAsync(connectionId, ct);
            return TypedResults.Ok(records.Select(x => new DnsRecordResponse(
                Guid.Empty,
                Guid.Empty,
                x.Name,
                DnsRecordTypeMapping.ToApiName(x.Type),
                x.Value,
                x.Ttl,
                x.IsManagedByHashi ? "managed" : "unknown",
                true,
                false,
                null,
                false,
                null)));
        })
            .Produces<IEnumerable<DnsRecordResponse>>(StatusCodes.Status200OK);

        group.MapGet("/zones", async (DnsRecordService records, CancellationToken ct) =>
        {
            var zones = await records.ListZonesAsync(ct);
            return TypedResults.Ok(zones.Select(x => new DnsZoneResponse(
                x.Id,
                x.ConnectionId,
                x.ProviderZoneId,
                x.Name,
                x.DefaultTtl)));
        })
            .Produces<IEnumerable<DnsZoneResponse>>(StatusCodes.Status200OK);

        group.MapPost("/connections/{connectionId:guid}/import/preview", async Task<IResult> (
            Guid connectionId,
            DnsConnectionService dns,
            CancellationToken ct) =>
        {
            var decisions = await dns.BuildImportPreviewAsync(connectionId, ct);
            return TypedResults.Ok(decisions.Select(x => new DnsImportDecisionResponse(
                x.Id, x.ProviderRecordId, x.Name, x.Type, x.Value, x.SelectedForImport)));
        })
            .Produces<IEnumerable<DnsImportDecisionResponse>>(StatusCodes.Status200OK);

        group.MapPost("/connections/{connectionId:guid}/import/apply", async Task<IResult> (
            Guid connectionId,
            DnsImportApplyRequest request,
            DnsConnectionService dns,
            SyncOrchestratorService sync,
            CancellationToken ct) =>
        {
            await dns.ApplyImportAsync(connectionId, request.SelectedDecisionIds, ct);
            await sync.TriggerImmediateSyncAsync(ct);
            return TypedResults.Ok(new { applied = true });
        });

        group.MapPost("/connections/{connectionId:guid}/prune/preview", async Task<IResult> (
            Guid connectionId,
            DnsConnectionService dns,
            CancellationToken ct) =>
        {
            var plan = await dns.BuildPrunePreviewAsync(connectionId, ct);
            return TypedResults.Ok(ToPlan(plan));
        })
            .Produces<DnsSyncPlanResponse>(StatusCodes.Status200OK);

        group.MapPost("/connections/{connectionId:guid}/prune/apply", async Task<IResult> (
            Guid connectionId,
            DnsPruneApplyRequest request,
            DnsConnectionService dns,
            SyncOrchestratorService sync,
            CancellationToken ct) =>
        {
            if (!request.ConfirmDestructive)
            {
                return TypedResults.BadRequest(new ApiErrorResponse("Destructive prune requires confirmation."));
            }

            await dns.ApplyPruneAsync(connectionId, request.ConfirmDestructive, ct);
            await sync.TriggerImmediateSyncAsync(ct);
            return TypedResults.Ok(new { applied = true });
        });

        group.MapPost("/connections/{connectionId:guid}/sync/plan", async Task<IResult> (
            Guid connectionId,
            DnsConnectionService dns,
            CancellationToken ct) =>
        {
            var plan = await dns.PlanSyncAsync(connectionId, ct);
            return TypedResults.Ok(ToPlan(plan));
        })
            .Produces<DnsSyncPlanResponse>(StatusCodes.Status200OK);

        group.MapPost("/connections/{connectionId:guid}/sync/apply", async Task<IResult> (
            Guid connectionId,
            DnsSyncApplyRequest request,
            DnsConnectionService dns,
            SyncOrchestratorService sync,
            CancellationToken ct) =>
        {
            if (request.ConnectionId != connectionId)
            {
                return TypedResults.BadRequest(new ApiErrorResponse("ConnectionId mismatch."));
            }

            var plan = await dns.PlanSyncAsync(connectionId, ct);
            if (plan.PlanId != request.PlanId)
            {
                return TypedResults.BadRequest(new ApiErrorResponse("Plan is stale. Re-run sync/plan."));
            }

            var syncRunId = await dns.ApplyPlanWithSyncRunAsync(plan, request.ConfirmDestructive, ct);
            await sync.TriggerImmediateSyncAsync(ct);
            return TypedResults.Ok(new { applied = true, syncRunId });
        });

        DnsRecordEndpoints.Map(group);

        return app;
    }

    private static ConnectionSummaryResponse ToSummary(ConnectionEntity connection)
        => new(
            connection.Id,
            connection.Name,
            connection.Type,
            connection.Enabled,
            connection.HealthState,
            connection.LastValidationMessage,
            connection.LastValidatedAtUtc);

    private static DnsSyncPlanResponse ToPlan(Hashi.Core.Dns.DnsSyncPlan plan)
        => new(
            plan.PlanId,
            plan.ConnectionId,
            plan.ZoneName,
            plan.Changes.Select(x => new DnsPlanChangeResponse(
                x.Kind.ToString(),
                x.Name,
                DnsRecordTypeMapping.ToApiName(x.Type),
                x.CurrentValue,
                x.DesiredValue,
                x.Ttl,
                x.RiskReason)).ToList(),
            plan.RequiresConfirmation);
}
