using Hashi.Contracts.Api;
using Hashi.Infrastructure.Platform;
using Microsoft.AspNetCore.Builder;
using Microsoft.AspNetCore.Http;
using Microsoft.AspNetCore.Routing;

namespace Hashi.Api.Features.Public;

public static class PublicEndpoints
{
    public static IEndpointRouteBuilder MapPublicEndpoints(this IEndpointRouteBuilder app)
    {
        app.MapGet("/api/public/status", async Task<IResult> (int? hours, MonitoringService monitoring, CancellationToken ct) =>
        {
            if (!await monitoring.IsPublicStatusEnabledAsync(ct))
            {
                return TypedResults.NotFound();
            }

            return TypedResults.Ok(await monitoring.PublicStatusAsync(hours, ct));
        })
            .WithTags("Public")
            .AllowAnonymous()
            .RequireCors("PublicRead")
            .Produces<IEnumerable<PublicStatusItemResponse>>(StatusCodes.Status200OK)
            .Produces(StatusCodes.Status404NotFound);
        app.MapGet("/api/public/status/summary", async Task<IResult> (MonitoringService monitoring, CancellationToken ct) =>
        {
            if (!await monitoring.IsPublicStatusEnabledAsync(ct))
            {
                return TypedResults.NotFound();
            }

            return TypedResults.Ok(await monitoring.PublicSummaryAsync(ct));
        })
            .WithTags("Public")
            .AllowAnonymous()
            .RequireCors("PublicRead")
            .Produces<PublicStatusSummaryResponse>(StatusCodes.Status200OK)
            .Produces(StatusCodes.Status404NotFound);
        app.MapGet("/api/public/apps", async Task<IResult> (PublicDashboardService dashboard, CancellationToken ct) =>
        {
            if (!await dashboard.IsPublicDashboardEnabledAsync(ct))
            {
                return TypedResults.NotFound();
            }

            return TypedResults.Ok(await dashboard.GetAsync(ct));
        })
            .WithTags("Public")
            .AllowAnonymous()
            .RequireCors("PublicRead")
            .Produces<PublicDashboardResponse>(StatusCodes.Status200OK)
            .Produces(StatusCodes.Status404NotFound);
        return app;
    }
}
