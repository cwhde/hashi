using Hashi.Contracts.Api;
using Hashi.Infrastructure.Bootstrap;
using Hashi.Infrastructure.Services;
using Microsoft.AspNetCore.Http.HttpResults;

namespace Hashi.Api.Features.Setup;

public static class SetupEndpoints
{
    public static IEndpointRouteBuilder MapSetupEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app.MapGroup("/api/setup").WithTags("Setup");

        group.MapGet("/status", async (SetupStateService setup, CancellationToken ct) =>
        {
            var state = await setup.GetOrCreateAsync(ct);
            var completed = await setup.GetCompletedStepsAsync(ct);
            return TypedResults.Ok(new SetupStatusResponse(
                state.IsComplete,
                state.CurrentStep,
                completed,
                state.UpdatedAtUtc));
        });

        group.MapGet("/bootstrap-allowed", (HttpContext httpContext) =>
        {
            var remoteIp = httpContext.Connection.RemoteIpAddress?.ToString();
            return TypedResults.Ok(new { allowed = BootstrapNetworkPolicy.IsAllowed(remoteIp), remoteIp });
        });

        return app;
    }
}

public static class ActivityEndpoints
{
    public static IEndpointRouteBuilder MapActivityEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app.MapGroup("/api/activity").WithTags("Activity");

        group.MapGet("/audit", async (AuditService audit, CancellationToken ct) =>
        {
            var events = await audit.ListRecentAsync(cancellationToken: ct);
            return TypedResults.Ok(events.Select(x => new AuditEventResponse(
                x.Id,
                x.Category,
                x.Action,
                x.SubjectType,
                x.SubjectId,
                x.Outcome,
                x.CreatedAtUtc)));
        });

        return app;
    }
}

public static class HealthEndpoints
{
    public static IEndpointRouteBuilder MapHealthEndpoints(this IEndpointRouteBuilder app)
    {
        app.MapGet("/api/health", () => TypedResults.Ok(new HealthResponse("healthy", "2.0.0-alpha", DateTimeOffset.UtcNow)))
            .WithTags("Health")
            .AllowAnonymous();

        return app;
    }
}
