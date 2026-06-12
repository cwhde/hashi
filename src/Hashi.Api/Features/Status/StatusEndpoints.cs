using Hashi.Contracts.Api;
using Hashi.Infrastructure.Platform;
using Microsoft.AspNetCore.Http;
using Microsoft.AspNetCore.Routing;

namespace Hashi.Api.Features.Status;

public static class StatusEndpoints
{
    public static IEndpointRouteBuilder MapStatusEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app.MapGroup("/api/status").WithTags("Status");
        group.MapGet("/endpoints", async (MonitoringService monitoring, CancellationToken ct) =>
        {
            var items = await monitoring.ListResponsesAsync(ct);
            return TypedResults.Ok(items);
        });
        group.MapPost("/endpoints", async Task<IResult> (
            CreateMonitorEndpointRequest request,
            MonitoringService monitoring,
            CancellationToken ct) =>
        {
            try
            {
                return TypedResults.Ok(MonitoringService.ToResponse(await monitoring.CreateManualAsync(request, ct)));
            }
            catch (InvalidOperationException ex)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(ex.Message));
            }
        })
            .Produces<MonitorEndpointResponse>(StatusCodes.Status200OK);
        group.MapPut("/endpoints/{endpointId:guid}", async Task<IResult> (
            Guid endpointId,
            UpdateMonitorEndpointRequest request,
            MonitoringService monitoring,
            CancellationToken ct) =>
        {
            try
            {
                var updated = await monitoring.UpdateManualAsync(endpointId, request, ct);
                return updated is null ? TypedResults.NotFound() : TypedResults.Ok(MonitoringService.ToResponse(updated));
            }
            catch (InvalidOperationException ex)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(ex.Message));
            }
        })
            .Produces<MonitorEndpointResponse>(StatusCodes.Status200OK);
        group.MapDelete("/endpoints/{endpointId:guid}", async Task<IResult> (
            Guid endpointId,
            MonitoringService monitoring,
            CancellationToken ct) =>
        {
            var deleted = await monitoring.DeleteManualAsync(endpointId, ct);
            return deleted ? TypedResults.NoContent() : TypedResults.NotFound();
        });
        group.MapGet("/rollups", async Task<IResult> (
            Guid? endpointId,
            int? intervalMinutes,
            int? hours,
            MonitoringService monitoring,
            CancellationToken ct) =>
        {
            var rollups = await monitoring.ListRollupsAsync(
                endpointId,
                intervalMinutes,
                hours ?? 1,
                ct);
            return TypedResults.Ok(rollups.Select(MonitoringService.ToRollupResponse));
        })
            .Produces<IEnumerable<MonitorRollupResponse>>(StatusCodes.Status200OK);
        group.MapGet("/events", async Task<IResult> (
            Guid? endpointId,
            int? hours,
            MonitoringService monitoring,
            CancellationToken ct) =>
        {
            var events = await monitoring.ListEventsAsync(endpointId, hours ?? 24, ct);
            return TypedResults.Ok(events.Select(MonitoringService.ToEventResponse));
        })
            .Produces<IEnumerable<MonitorEventResponse>>(StatusCodes.Status200OK);
        return app;
    }
}
