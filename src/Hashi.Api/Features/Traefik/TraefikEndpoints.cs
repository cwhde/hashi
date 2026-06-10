using Hashi.Api.Hosting;
using Hashi.Contracts.Api;
using Hashi.Core.Traefik;
using Hashi.Infrastructure.Platform;
using Microsoft.AspNetCore.Http;
using Microsoft.AspNetCore.Routing;

namespace Hashi.Api.Features.Traefik;

public static class TraefikEndpoints
{
    public static IEndpointRouteBuilder MapTraefikEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app.MapGroup("/api/traefik").WithTags("Traefik");
        group.MapGet("/render", async (TraefikPlatformService traefik, CancellationToken ct) =>
        {
            var result = await traefik.RenderAsync(ct);
            return TypedResults.Ok(new TraefikRenderResponse(
                result.StaticConfigYaml,
                result.DynamicFiles.HttpResourcesYaml,
                result.ContentHash,
                new TraefikDynamicFilesResponse(
                    result.DynamicFiles.CoreYaml,
                    result.DynamicFiles.HttpResourcesYaml,
                    result.DynamicFiles.StreamResourcesYaml,
                    result.DynamicFiles.UserMiddlewaresYaml,
                    result.DynamicFiles.SecurityYaml,
                    result.DynamicFiles.HealthYaml)));
        })
            .Produces<TraefikRenderResponse>(StatusCodes.Status200OK);
        group.MapPost("/validate", async (TraefikPlatformService traefik, CancellationToken ct) =>
        {
            var render = await traefik.RenderAsync(ct);
            var validation = TraefikConfigValidator.ValidateRender(render);
            return TypedResults.Ok(new TraefikConfigValidationResponse(validation.IsValid, validation.Errors));
        })
            .Produces<TraefikConfigValidationResponse>(StatusCodes.Status200OK);
        group.MapGet("/user-middlewares", async (TraefikUserMiddlewareService middlewares, CancellationToken ct) =>
            TypedResults.Ok(await middlewares.GetAsync(ct)))
            .Produces<TraefikUserMiddlewareResponse>(StatusCodes.Status200OK);
        group.MapPut("/user-middlewares", async Task<IResult> (
            UpdateTraefikUserMiddlewareRequest request,
            TraefikUserMiddlewareService middlewares,
            CancellationToken ct) =>
        {
            try
            {
                return TypedResults.Ok(await middlewares.UpdateAsync(request, ct));
            }
            catch (InvalidOperationException ex)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(ex.Message));
            }
        })
            .Produces<TraefikUserMiddlewareResponse>(StatusCodes.Status200OK);
        group.MapPost("/user-middlewares/validate", (TraefikUserMiddlewareValidationRequest request, TraefikUserMiddlewareService middlewares) =>
            TypedResults.Ok(middlewares.Validate(request.Yaml)))
            .Produces<TraefikUserMiddlewareValidationResponse>(StatusCodes.Status200OK);
        group.MapGet("/connections/{connectionId:guid}/state", async (
            Guid connectionId,
            TraefikPlatformService traefik,
            CancellationToken ct) =>
            TypedResults.Ok(await traefik.GetHostStateAsync(connectionId, ct)))
            .Produces<TraefikHostStateResponse>(StatusCodes.Status200OK);
        group.MapPost("/connections/{connectionId:guid}/detect-existing", async (
            Guid connectionId,
            TraefikSyncService sync,
            CancellationToken ct) =>
            TypedResults.Ok(await sync.DetectExistingAsync(connectionId, ct)))
            .Produces<TraefikDetectExistingResponse>(StatusCodes.Status200OK);
        group.MapPost("/connections/{connectionId:guid}/apply", async (
            Guid connectionId,
            TraefikApplyConnectionRequest request,
            TraefikSyncService sync,
            CancellationToken ct) =>
            TypedResults.Ok(await sync.ApplyForConnectionAsync(connectionId, request.ConfirmReplaceExisting, ct)))
            .Produces<TraefikApplyResponse>(StatusCodes.Status200OK);
        group.MapPost("/connections/{connectionId:guid}/rollback", async (
            Guid connectionId,
            TraefikSyncService sync,
            CancellationToken ct) =>
            TypedResults.Ok(await sync.RollbackForConnectionAsync(connectionId, ct)))
            .Produces<TraefikApplyResponse>(StatusCodes.Status200OK);
        group.MapPost("/apply", async Task<IResult> (TraefikApplyRequest request, TraefikSyncService sync, CancellationToken ct) =>
        {
            var result = await sync.ApplyAsync(request, ct);
            return TypedResults.Ok(result);
        })
            .Produces<TraefikApplyResponse>(StatusCodes.Status200OK);
        group.MapPost("/rollback", async Task<IResult> (TraefikApplyRequest request, TraefikSyncService sync, CancellationToken ct) =>
        {
            var result = await sync.RollbackAsync(request, ct);
            return TypedResults.Ok(result);
        })
            .Produces<TraefikApplyResponse>(StatusCodes.Status200OK);
        group.MapPost("/install", async Task<IResult> (TraefikInstallRequest request, TraefikSyncService sync, CancellationToken ct) =>
        {
            var result = await sync.InstallAsync(request, ct);
            return TypedResults.Ok(result);
        })
            .Produces<TraefikInstallResponse>(StatusCodes.Status200OK);
        group.MapGet("/entrypoints", async (TraefikEntryPointService entryPoints, CancellationToken ct) =>
            TypedResults.Ok(await entryPoints.ListAllAsync(ct)))
            .Produces<IEnumerable<TraefikEntryPointResponse>>(StatusCodes.Status200OK);
        group.MapGet("/entrypoints/pending", async (TraefikEntryPointService entryPoints, CancellationToken ct) =>
            TypedResults.Ok(await entryPoints.ListPendingAsync(ct)))
            .Produces<IEnumerable<TraefikEntryPointResponse>>(StatusCodes.Status200OK);
        group.MapPost("/entrypoints/{entryPointId:guid}/confirm", async Task<IResult> (
            Guid entryPointId,
            TraefikEntryPointService entryPoints,
            CancellationToken ct) =>
        {
            var confirmed = await entryPoints.ConfirmAsync(entryPointId, ct);
            return confirmed is null ? TypedResults.NotFound() : TypedResults.Ok(confirmed);
        })
            .Produces<TraefikEntryPointResponse>(StatusCodes.Status200OK);
        return app;
    }
}
