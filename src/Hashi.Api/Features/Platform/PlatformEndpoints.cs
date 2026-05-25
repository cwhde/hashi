using Hashi.Contracts.Api;
using Hashi.Infrastructure.Platform;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Microsoft.AspNetCore.Http.HttpResults;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Api.Features.Resources;

public static class ResourceEndpoints
{
    public static IEndpointRouteBuilder MapResourceEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app.MapGroup("/api/resources").WithTags("Resources");

        group.MapGet("/", async (ResourceService resources, CancellationToken ct) =>
        {
            var items = await resources.ListAsync(ct);
            return TypedResults.Ok(items.Select(ResourceService.ToResponse));
        });

        group.MapPost("/", async Task<IResult> (CreateResourceRequest request, ResourceService resources, CancellationToken ct) =>
        {
            var created = await resources.CreateAsync(request, ct);
            return TypedResults.Ok(ResourceService.ToResponse(created));
        });

        group.MapPut("/{id:guid}", async Task<IResult> (Guid id, UpdateResourceRequest request, ResourceService resources, CancellationToken ct) =>
        {
            try
            {
                var updated = await resources.UpdateAsync(id, request, ct);
                return updated is null ? TypedResults.NotFound() : TypedResults.Ok(ResourceService.ToResponse(updated));
            }
            catch (InvalidOperationException ex)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(ex.Message));
            }
        });

        group.MapDelete("/{id:guid}", async Task<IResult> (Guid id, ResourceService resources, CancellationToken ct) =>
        {
            try
            {
                var deleted = await resources.DeleteAsync(id, ct);
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

public static class TraefikEndpoints
{
    public static IEndpointRouteBuilder MapTraefikEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app.MapGroup("/api/traefik").WithTags("Traefik");
        group.MapGet("/render", async (TraefikPlatformService traefik, CancellationToken ct) =>
        {
            var result = await traefik.RenderAsync(ct);
            return TypedResults.Ok(new TraefikRenderResponse(result.StaticConfigYaml, result.DynamicHttpYaml, result.ContentHash));
        });
        return app;
    }
}

public static class FirewallEndpoints
{
    public static IEndpointRouteBuilder MapFirewallEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app.MapGroup("/api/firewall").WithTags("Firewall");
        group.MapPost("/render", (FirewallRenderRequest request, FirewallPlatformService firewall) =>
            TypedResults.Ok(firewall.Render(request)));
        return app;
    }
}

public static class StatusEndpoints
{
    public static IEndpointRouteBuilder MapStatusEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app.MapGroup("/api/status").WithTags("Status");
        group.MapGet("/endpoints", async (MonitoringService monitoring, CancellationToken ct) =>
        {
            var items = await monitoring.ListAsync(ct);
            return TypedResults.Ok(items.Select(x => new MonitorEndpointResponse(
                x.Id, x.Name, x.Url, x.CheckType, x.Enabled, x.Status, x.LastCheckedAtUtc, x.LastLatencyMs)));
        });
        return app;
    }
}

public static class PublicEndpoints
{
    public static IEndpointRouteBuilder MapPublicEndpoints(this IEndpointRouteBuilder app)
    {
        app.MapGet("/api/public/status", async (MonitoringService monitoring, CancellationToken ct) =>
            TypedResults.Ok(await monitoring.PublicStatusAsync(ct))).WithTags("Public").AllowAnonymous();
        app.MapGet("/api/public/apps", async (ResourceService resources, CancellationToken ct) =>
        {
            var items = await resources.ListAsync(ct);
            return TypedResults.Ok(items.Where(x => x.DashboardEnabled).Select(ResourceService.ToResponse));
        }).WithTags("Public").AllowAnonymous();
        return app;
    }
}

public static class EdgeAuthEndpoints
{
    public static IEndpointRouteBuilder MapEdgeAuthEndpoints(this IEndpointRouteBuilder app)
    {
        app.MapGet("/api/edge-auth/forward", (HttpContext ctx) =>
        {
            var host = ctx.Request.Headers["X-Forwarded-Host"].FirstOrDefault() ?? ctx.Request.Host.Value;
            return TypedResults.Ok(new EdgeAuthForwardResponse("allow", null));
        }).WithTags("EdgeAuth").AllowAnonymous();
        return app;
    }
}

public static class SecurityEndpoints
{
    public static IEndpointRouteBuilder MapSecurityEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app.MapGroup("/api/security").WithTags("Security");
        group.MapGet("/dashboard", () => TypedResults.Ok(new SecurityDashboardResponse(0, 0, 0, Array.Empty<string>())));
        return app;
    }
}

public static class PulseEndpoints
{
    public static IEndpointRouteBuilder MapPulseEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app.MapGroup("/api/pulse").WithTags("Pulse");
        group.MapGet("/agents", async (HashiDbContext db, CancellationToken ct) =>
        {
            var agents = await db.PulseAgents.AsNoTracking().ToListAsync(ct);
            return TypedResults.Ok(agents.Select(x => new PulseAgentResponse(x.Id, x.Name, x.Status, x.LastSeenAtUtc, x.LastPublicIp)));
        });
        group.MapPost("/{agentId:guid}/heartbeat", async Task<IResult> (
            Guid agentId,
            PulseHeartbeatRequest request,
            HashiDbContext db,
            CancellationToken ct) =>
        {
            var agent = await db.PulseAgents.SingleOrDefaultAsync(x => x.Id == agentId, ct);
            if (agent is null)
            {
                return TypedResults.NotFound();
            }

            agent.LastSeenAtUtc = DateTimeOffset.UtcNow;
            agent.LastPublicIp = request.PrivateIpv4Candidates.FirstOrDefault();
            agent.Status = "online";
            await db.SaveChangesAsync(ct);
            return TypedResults.Ok(new { accepted = true });
        }).AllowAnonymous();
        return app;
    }
}

public static class ScriptEndpoints
{
    public static IEndpointRouteBuilder MapScriptEndpoints(this IEndpointRouteBuilder app)
    {
        app.MapGet("/api/scripts", () => TypedResults.Ok(Array.Empty<ScriptResponse>())).WithTags("Scripts");
        return app;
    }
}

public static class NotificationEndpoints
{
    public static IEndpointRouteBuilder MapNotificationEndpoints(this IEndpointRouteBuilder app)
    {
        app.MapGet("/api/settings/notifications/providers", () =>
            TypedResults.Ok(Array.Empty<NotificationProviderResponse>())).WithTags("Settings");
        return app;
    }
}

public static class AdGuardEndpoints
{
    public static IEndpointRouteBuilder MapAdGuardEndpoints(this IEndpointRouteBuilder app)
    {
        app.MapGet("/api/adguard/rewrites", () => TypedResults.Ok(Array.Empty<object>())).WithTags("AdGuard");
        return app;
    }
}
