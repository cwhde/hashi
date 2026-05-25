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

        group.MapGet("/connections/{connectionId:guid}/records/provider", async Task<IResult> (
            Guid connectionId,
            DnsConnectionService dns,
            CancellationToken ct) =>
        {
            var records = await dns.ListProviderRecordsAsync(connectionId, ct);
            return TypedResults.Ok(records.Select(x => new DnsRecordResponse(
                Guid.Empty,
                x.Name,
                DnsRecordTypeMapping.ToApiName(x.Type),
                x.Value,
                x.Ttl,
                x.IsManagedByHashi ? "managed" : "unknown",
                true)));
        });

        group.MapPost("/connections/{connectionId:guid}/import/preview", async Task<IResult> (
            Guid connectionId,
            DnsConnectionService dns,
            CancellationToken ct) =>
        {
            var decisions = await dns.BuildImportPreviewAsync(connectionId, ct);
            return TypedResults.Ok(decisions.Select(x => new DnsImportDecisionResponse(
                x.Id, x.ProviderRecordId, x.Name, x.Type, x.Value, x.SelectedForImport)));
        });

        group.MapPost("/connections/{connectionId:guid}/import/apply", async Task<IResult> (
            Guid connectionId,
            DnsImportApplyRequest request,
            DnsConnectionService dns,
            CancellationToken ct) =>
        {
            await dns.ApplyImportAsync(connectionId, request.SelectedDecisionIds, ct);
            return TypedResults.Ok(new { applied = true });
        });

        group.MapPost("/connections/{connectionId:guid}/sync/plan", async Task<IResult> (
            Guid connectionId,
            DnsConnectionService dns,
            CancellationToken ct) =>
        {
            var plan = await dns.PlanSyncAsync(connectionId, ct);
            return TypedResults.Ok(ToPlan(plan));
        });

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

            await dns.ApplyPlanAsync(plan, request.ConfirmDestructive, ct);
            return TypedResults.Ok(new { applied = true });
        });

        group.MapGet("/records", async (HashiDbContext db, CancellationToken ct) =>
        {
            var records = await db.DnsRecords.AsNoTracking().OrderBy(x => x.Name).ToListAsync(ct);
            return TypedResults.Ok(records.Select(x => new DnsRecordResponse(
                x.Id, x.Name, x.Type, x.Value, x.Ttl, x.Ownership, x.Enabled)));
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
