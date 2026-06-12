using Hashi.Api.Hosting;
using Hashi.Contracts.Api;
using Hashi.Infrastructure.Platform;
using Microsoft.AspNetCore.Http;
using Microsoft.AspNetCore.Http.HttpResults;
using Microsoft.AspNetCore.Routing;

namespace Hashi.Api.Features.Script;

public static class ScriptEndpoints
{
    public static IEndpointRouteBuilder MapScriptEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app.MapGroup("/api/scripts").WithTags("Scripts");
        group.MapGet("/", async (ScriptExecutionService scripts, CancellationToken ct) =>
            TypedResults.Ok(await scripts.ListAsync(ct)));
        group.MapGet("/{scriptId:guid}", async Task<IResult> (Guid scriptId, ScriptExecutionService scripts, CancellationToken ct) =>
        {
            var script = await scripts.GetAsync(scriptId, ct);
            return script is null ? TypedResults.NotFound() : TypedResults.Ok(script);
        })
            .Produces<ScriptResponse>(StatusCodes.Status200OK);
        group.MapPost("/", async Task<IResult> (CreateScriptRequest request, ScriptExecutionService scripts, CancellationToken ct) =>
        {
            var created = await scripts.CreateAsync(request, ct);
            return TypedResults.Ok(created);
        })
            .Produces<ScriptResponse>(StatusCodes.Status200OK);
        group.MapPut("/{scriptId:guid}", async Task<IResult> (
            Guid scriptId,
            UpdateScriptRequest request,
            ScriptExecutionService scripts,
            CancellationToken ct) =>
        {
            var updated = await scripts.UpdateAsync(scriptId, request, ct);
            return updated is null ? TypedResults.NotFound() : TypedResults.Ok(updated);
        })
            .Produces<ScriptResponse>(StatusCodes.Status200OK);
        group.MapDelete("/{scriptId:guid}", async Task<IResult> (Guid scriptId, ScriptExecutionService scripts, CancellationToken ct) =>
        {
            var deleted = await scripts.DeleteAsync(scriptId, ct);
            return deleted ? TypedResults.NoContent() : TypedResults.NotFound();
        });
        group.MapPost("/{scriptId:guid}/run", async Task<IResult> (
            Guid scriptId,
            RunScriptRequest request,
            ScriptExecutionService scripts,
            CancellationToken ct) =>
        {
            var result = await scripts.RunAsync(scriptId, request, ct);
            return TypedResults.Ok(result);
        })
            .Produces<RunScriptResponse>(StatusCodes.Status200OK);
        return app;
    }
}
