using Hashi.Api.Hosting;
using Hashi.Contracts.Api;
using Hashi.Infrastructure.Platform;
using Microsoft.AspNetCore.Http;
using Microsoft.AspNetCore.Http.HttpResults;
using Microsoft.AspNetCore.Routing;

namespace Hashi.Api.Features.AdGuard;

public static class AdGuardEndpoints
{
    public static IEndpointRouteBuilder MapAdGuardEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app.MapGroup("/api/adguard").WithTags("AdGuard");
        group.MapGet("/connections", async (AdGuardSyncService adguard, CancellationToken ct) =>
            TypedResults.Ok(await adguard.ListConnectionsAsync(ct)))
            .Produces<IEnumerable<AdGuardConnectionResponse>>(StatusCodes.Status200OK);
        group.MapPost("/connections", async Task<IResult> (
            CreateAdGuardConnectionRequest request,
            AdGuardSyncService adguard,
            CancellationToken ct) =>
        {
            try
            {
                return TypedResults.Ok(await adguard.CreateConnectionAsync(request, ct));
            }
            catch (InvalidOperationException ex)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(ex.Message));
            }
        })
            .Produces<AdGuardConnectionResponse>(StatusCodes.Status200OK);
        group.MapPost("/connections/{connectionId:guid}/test", async Task<IResult> (
            Guid connectionId,
            AdGuardSyncService adguard,
            CancellationToken ct) =>
            TypedResults.Ok(await adguard.TestConnectionAsync(connectionId, ct)))
            .Produces<AdGuardConnectionTestResponse>(StatusCodes.Status200OK);
        group.MapGet("/{connectionId:guid}/rewrites", async Task<IResult> (
            Guid connectionId,
            AdGuardSyncService adguard,
            CancellationToken ct) => TypedResults.Ok(await adguard.ListRewritesAsync(connectionId, ct)))
            .Produces<IEnumerable<AdGuardRewriteResponse>>(StatusCodes.Status200OK);
        group.MapPost("/{connectionId:guid}/rewrites", async Task<IResult> (
            Guid connectionId,
            UpsertAdGuardRewriteRequest request,
            AdGuardSyncService adguard,
            CancellationToken ct) =>
        {
            try
            {
                var rewrite = await adguard.UpsertRewriteAsync(connectionId, request, ct);
                return TypedResults.Ok(rewrite);
            }
            catch (InvalidOperationException ex)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(ex.Message));
            }
        })
            .Produces<AdGuardRewriteMutationResponse>(StatusCodes.Status200OK);
        group.MapDelete("/{connectionId:guid}/rewrites/{rewriteId:guid}", async Task<IResult> (
            Guid connectionId,
            Guid rewriteId,
            AdGuardSyncService adguard,
            CancellationToken ct) =>
        {
            try
            {
                var plan = await adguard.DeleteRewriteAsync(connectionId, rewriteId, ct);
                return plan is null ? TypedResults.NotFound() : TypedResults.Ok(plan);
            }
            catch (InvalidOperationException ex)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(ex.Message));
            }
        })
            .Produces<AdGuardRewriteMutationResponse>(StatusCodes.Status200OK);
        group.MapPost("/{connectionId:guid}/rewrites/{rewriteId:guid}/delete/apply", async Task<IResult> (
            Guid connectionId,
            Guid rewriteId,
            AdGuardRewriteApplyRequest request,
            AdGuardSyncService adguard,
            CancellationToken ct) =>
        {
            try
            {
                return TypedResults.Ok(await adguard.ApplyPlanAsync(connectionId, request, deleteRewriteId: rewriteId, cancellationToken: ct));
            }
            catch (InvalidOperationException ex)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(ex.Message));
            }
        })
            .Produces<AdGuardRewriteApplyResponse>(StatusCodes.Status200OK);
        group.MapPost("/{connectionId:guid}/sync/plan", async Task<IResult> (
            Guid connectionId,
            AdGuardSyncService adguard,
            CancellationToken ct) => TypedResults.Ok(await adguard.PlanSyncAsync(
                connectionId,
                updateTopologyDesiredState: true,
                updateInternalAgentDnsDesiredState: true,
                cancellationToken: ct)))
            .Produces<AdGuardRewritePlanResponse>(StatusCodes.Status200OK);
        group.MapPost("/{connectionId:guid}/sync/apply", async Task<IResult> (
            Guid connectionId,
            AdGuardRewriteApplyRequest request,
            AdGuardSyncService adguard,
            CancellationToken ct) =>
        {
            try
            {
                return TypedResults.Ok(await adguard.ApplyPlanAsync(
                    connectionId,
                    request,
                    updateTopologyDesiredState: true,
                    updateInternalAgentDnsDesiredState: true,
                    cancellationToken: ct));
            }
            catch (InvalidOperationException ex)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(ex.Message));
            }
        })
            .Produces<AdGuardRewriteApplyResponse>(StatusCodes.Status200OK);
        group.MapPost("/{connectionId:guid}/sync", async Task<IResult> (
            Guid connectionId,
            AdGuardSyncService adguard,
            CancellationToken ct) => TypedResults.Ok(await adguard.SyncManagedRewritesAsync(connectionId, cancellationToken: ct)))
            .Produces<AdGuardRewriteApplyResponse>(StatusCodes.Status200OK);
        return app;
    }
}
