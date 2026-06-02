using Hashi.Contracts.Api;
using Hashi.Core.Dns;
using Hashi.Infrastructure.Dns;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
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
            CancellationToken ct) =>
        {
            var connection = await dns.CreateHetznerConnectionAsync(
                request.Name,
                request.ApiToken,
                request.ZoneName,
                request.DefaultTtl,
                ct);
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
            CancellationToken ct) =>
        {
            await dns.ApplyImportAsync(connectionId, request.SelectedDecisionIds, ct);
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
            CancellationToken ct) =>
        {
            if (!request.ConfirmDestructive)
            {
                return TypedResults.BadRequest(new ApiErrorResponse("Destructive prune requires confirmation."));
            }

            await dns.ApplyPruneAsync(connectionId, request.ConfirmDestructive, ct);
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
            return TypedResults.Ok(new { applied = true, syncRunId });
        });

        group.MapGet("/records", async (DnsRecordService records, CancellationToken ct) =>
        {
            var items = await records.ListAsync(ct);
            return TypedResults.Ok(items.Select(ToRecord));
        });

        group.MapPost("/records", async Task<IResult> (
            UpsertDnsRecordRequest request,
            DnsRecordService records,
            CancellationToken ct) =>
        {
            try
            {
                return TypedResults.Ok(ToRecord(await records.CreateManualAsync(
                    request.ZoneId,
                    request.Name,
                    request.Type,
                    request.Value,
                    request.Ttl,
                    request.Enabled,
                    request.DashboardEnabled,
                    request.DashboardDisplayName,
                    ct)));
            }
            catch (InvalidOperationException ex)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(ex.Message));
            }
        });

        group.MapPut("/records/{recordId:guid}", async Task<IResult> (
            Guid recordId,
            UpsertDnsRecordRequest request,
            DnsRecordService records,
            CancellationToken ct) =>
        {
            try
            {
                var updated = await records.UpdateManualAsync(
                    recordId,
                    request.ZoneId,
                    request.Name,
                    request.Type,
                    request.Value,
                    request.Ttl,
                    request.Enabled,
                    request.DashboardEnabled,
                    request.DashboardDisplayName,
                    ct);
                return updated is null
                    ? TypedResults.NotFound(new ApiErrorResponse("Manual DNS record not found."))
                    : TypedResults.Ok(ToRecord(updated));
            }
            catch (InvalidOperationException ex)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(ex.Message));
            }
        });

        group.MapDelete("/records/{recordId:guid}", async Task<IResult> (
            Guid recordId,
            DnsRecordService records,
            CancellationToken ct) =>
        {
            var deleted = await records.DeleteManualAsync(recordId, ct);
            return deleted
                ? TypedResults.NoContent()
                : TypedResults.NotFound(new ApiErrorResponse("Manual DNS record not found."));
        });

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

    private static DnsRecordResponse ToRecord(DnsRecordEntity record)
        => new(
            record.Id,
            record.ZoneId,
            record.Name,
            record.Type,
            record.Value,
            record.Ttl,
            record.Ownership,
            record.Enabled,
            record.DashboardEnabled,
            record.DashboardDisplayName);

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
