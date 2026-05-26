using Hashi.Contracts.Api;
using Hashi.Infrastructure.Sync;
using Microsoft.AspNetCore.Http.HttpResults;

namespace Hashi.Api.Features.Sync;

public static class SyncEndpoints
{
    public static IEndpointRouteBuilder MapSyncEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app.MapGroup("/api/sync").WithTags("Sync");

        group.MapGet("/runs", async (SyncRunService sync, CancellationToken ct) =>
        {
            var runs = await sync.ListRecentAsync(50, ct);
            return TypedResults.Ok(runs);
        });

        group.MapGet("/runs/{id:guid}", async Task<IResult> (Guid id, SyncRunService sync, CancellationToken ct) =>
        {
            var run = await sync.GetAsync(id, ct);
            return run is null ? TypedResults.NotFound() : TypedResults.Ok(run);
        });

        group.MapPost("/plan", async (SyncOrchestratorService orchestrator, CancellationToken ct) =>
        {
            var plan = await orchestrator.PlanGlobalAsync(ct);
            return TypedResults.Ok(plan);
        });

        group.MapPost("/apply", async Task<IResult> (SyncApplyRequest request, SyncOrchestratorService orchestrator, CancellationToken ct) =>
        {
            var result = await orchestrator.ApplyGlobalAsync(request.ConfirmDestructive, ct);
            return TypedResults.Ok(result);
        });

        group.MapPost("/reconcile", async (SyncOrchestratorService orchestrator, CancellationToken ct) =>
        {
            var result = await orchestrator.ReconcileAsync(ct);
            return TypedResults.Ok(result);
        });

        return app;
    }
}
