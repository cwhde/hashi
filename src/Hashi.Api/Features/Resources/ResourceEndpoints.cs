using FluentValidation;
using Hashi.Api.Hosting;
using Hashi.Contracts.Api;
using Hashi.Infrastructure.Platform;
using Hashi.Infrastructure.Sync;
using Microsoft.AspNetCore.Http;
using Microsoft.AspNetCore.Http.HttpResults;
using Microsoft.AspNetCore.Routing;

namespace Hashi.Api.Features.Resources;

public static class ResourceEndpoints
{
    public static IEndpointRouteBuilder MapResourceEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app.MapGroup("/api/resources").WithTags("Resources");

        group.MapGet("/", async (ResourceService resources, CancellationToken ct) =>
        {
            var items = await resources.ListAsync(ct);
            var responses = new List<ResourceResponse>();
            foreach (var item in items)
            {
                responses.Add(await resources.ToResponseAsync(item, ct));
            }

            return TypedResults.Ok(responses);
        });

        group.MapPost("/", async Task<IResult> (
            CreateResourceRequest request,
            IValidator<CreateResourceRequest> validator,
            ResourceService resources,
            SyncOrchestratorService sync,
            CancellationToken ct) =>
        {
            var validationErrors = await validator!.ValidateRequestAsync(request, ct);
            if (validationErrors is not null)
            {
                return TypedResults.ValidationProblem(validationErrors);
            }

            try
            {
                var created = await resources.CreateAsync(request, ct);
                await sync.TriggerImmediateSyncAsync(ct);
                return TypedResults.Ok(await resources.ToResponseAsync(created, ct));
            }
            catch (InvalidOperationException ex)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(ex.Message));
            }
        });

        group.MapPut("/{id:guid}", async Task<IResult> (Guid id, UpdateResourceRequest request, ResourceService resources, SyncOrchestratorService sync, CancellationToken ct) =>
        {
            try
            {
                var updated = await resources.UpdateAsync(id, request, ct);
                if (updated is not null)
                {
                    await sync.TriggerImmediateSyncAsync(ct);
                }
                return updated is null
                    ? TypedResults.NotFound()
                    : TypedResults.Ok(await resources.ToResponseAsync(updated, ct));
            }
            catch (InvalidOperationException ex)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(ex.Message));
            }
        });

        group.MapDelete("/{id:guid}", async Task<IResult> (Guid id, ResourceService resources, SyncOrchestratorService sync, CancellationToken ct) =>
        {
            try
            {
                var deleted = await resources.DeleteAsync(id, ct);
                if (deleted)
                {
                    await sync.TriggerImmediateSyncAsync(ct);
                }
                return deleted ? TypedResults.NoContent() : TypedResults.NotFound();
            }
            catch (InvalidOperationException ex)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(ex.Message));
            }
        });

        return app;
    }
}
