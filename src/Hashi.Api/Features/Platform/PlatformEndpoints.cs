using Hashi.Contracts.Api;
using Hashi.Infrastructure.Notifications;
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
        });
        group.MapPost("/apply", async Task<IResult> (TraefikApplyRequest request, TraefikSyncService sync, CancellationToken ct) =>
        {
            var result = await sync.ApplyAsync(request, ct);
            return TypedResults.Ok(result);
        });
        group.MapPost("/rollback", async Task<IResult> (TraefikApplyRequest request, TraefikSyncService sync, CancellationToken ct) =>
        {
            var result = await sync.RollbackAsync(request, ct);
            return TypedResults.Ok(result);
        });
        group.MapPost("/install", async Task<IResult> (TraefikInstallRequest request, TraefikSyncService sync, CancellationToken ct) =>
        {
            var result = await sync.InstallAsync(request, ct);
            return TypedResults.Ok(result);
        });
        return app;
    }
}

public static class FirewallEndpoints
{
    public static IEndpointRouteBuilder MapFirewallEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app.MapGroup("/api/firewall").WithTags("Firewall");
        group.MapGet("/hosts", async (FirewallApplyService firewall, CancellationToken ct) =>
            TypedResults.Ok(await firewall.ListHostsAsync(ct)));
        group.MapPost("/render", (FirewallRenderRequest request, FirewallPlatformService firewall) =>
            TypedResults.Ok(firewall.Render(request)));
        group.MapPost("/hosts", async Task<IResult> (CreateFirewallHostRequest request, FirewallApplyService firewall, CancellationToken ct) =>
        {
            var host = await firewall.UpsertHostAsync(request, ct);
            return TypedResults.Ok(FirewallApplyService.ToResponse(host));
        })
            .Produces<FirewallHostResponse>(StatusCodes.Status200OK);
        group.MapPost("/apply", async Task<IResult> (FirewallApplyRequest request, FirewallApplyService firewall, CancellationToken ct) =>
        {
            var result = await firewall.ApplyAsync(request, ct);
            return TypedResults.Ok(result);
        });
        group.MapPost("/{firewallHostId:guid}/rollback", async Task<IResult> (
            Guid firewallHostId,
            FirewallApplyRequest request,
            FirewallApplyService firewall,
            CancellationToken ct) =>
        {
            var result = await firewall.RollbackAsync(firewallHostId, request, ct);
            return TypedResults.Ok(result);
        });
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
            return TypedResults.Ok(items.Select(MonitoringService.ToResponse));
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
        app.MapGet("/api/edge-auth/forward", async Task<IResult> (
            HttpContext ctx,
            EdgeAuthService edgeAuth,
            SecurityIngestionService security,
            CancellationToken ct) =>
        {
            var host = ctx.Request.Headers["X-Forwarded-Host"].FirstOrDefault() ?? ctx.Request.Host.Value ?? string.Empty;
            var path = ctx.Request.Headers["X-Forwarded-Uri"].FirstOrDefault()
                ?? ctx.Request.Headers["X-Original-URL"].FirstOrDefault()
                ?? ctx.Request.Path.Value
                ?? "/";
            var clientIp = ctx.Connection.RemoteIpAddress ?? System.Net.IPAddress.Loopback;
            var country = ctx.Request.Headers["X-Geo-Country"].FirstOrDefault();
            var asn = ctx.Request.Headers["X-Geo-Asn"].FirstOrDefault();
            var mode = ctx.Request.Query["mode"].FirstOrDefault();
            var result = await edgeAuth.EvaluateForwardAsync(
                host, path, clientIp, country, asn, ctx.Request.Cookies["hashi.edge.session"], mode, ct);

            await security.IngestForwardAuthDecisionAsync(new ForwardAuthDecisionIngestRequest(
                clientIp.ToString(),
                host,
                path,
                result.Decision,
                country,
                asn), ct);

            return result.Decision switch
            {
                "allow" => TypedResults.StatusCode(StatusCodes.Status204NoContent),
                "deny" => TypedResults.StatusCode(StatusCodes.Status403Forbidden),
                "redirect" or "challenge" => TypedResults.Redirect(result.RedirectUrl ?? "/api/edge-auth/login"),
                _ => TypedResults.StatusCode(StatusCodes.Status401Unauthorized),
            };
        }).WithTags("EdgeAuth").AllowAnonymous();

        app.MapGet("/api/edge-auth/login", async Task<IResult> (
            HttpContext ctx,
            Guid? providerId,
            string? returnUrl,
            OidcEdgeAuthService oidc,
            CancellationToken ct) =>
        {
            var providers = await oidc.ListProvidersAsync(ct);
            var provider = providerId is Guid id
                ? providers.FirstOrDefault(x => x.Id == id)
                : providers.FirstOrDefault();
            if (provider is null)
            {
                return TypedResults.BadRequest(new ApiErrorResponse("No enabled OIDC provider configured."));
            }

            var authorizationUrl = await oidc.BuildAuthorizationUrlAsync(ctx, provider.Id, returnUrl ?? "/", ct);
            return TypedResults.Redirect(authorizationUrl);
        }).WithTags("EdgeAuth").AllowAnonymous();

        app.MapGet("/api/edge-auth/callback", async Task<IResult> (
            HttpContext ctx,
            Guid providerId,
            string? code,
            string? state,
            OidcEdgeAuthService oidc,
            CancellationToken ct) =>
        {
            if (string.IsNullOrWhiteSpace(code))
            {
                return TypedResults.BadRequest(new ApiErrorResponse("Missing authorization code."));
            }

            var result = await oidc.CompleteCallbackAsync(ctx, providerId, code, state, ct);
            ctx.Response.Cookies.Append("hashi.edge.session", result.SessionKey, result.SessionCookie);
            return TypedResults.Redirect(result.ReturnUrl);
        }).WithTags("EdgeAuth").AllowAnonymous();

        app.MapPost("/api/edge-auth/logout", (HttpContext ctx) =>
        {
            var sessionKey = ctx.Request.Cookies["hashi.edge.session"];
            OidcEdgeAuthService.ClearSession(sessionKey);
            if (!string.IsNullOrWhiteSpace(sessionKey))
            {
                ctx.Response.Cookies.Delete("hashi.edge.session");
            }

            return TypedResults.Ok(new { loggedOut = true });
        }).WithTags("EdgeAuth").AllowAnonymous();

        return app;
    }
}

public static class SecurityEndpoints
{
    public static IEndpointRouteBuilder MapSecurityEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app.MapGroup("/api/security").WithTags("Security");
        group.MapGet("/dashboard", async (int? hours, SecurityIngestionService security, CancellationToken ct) =>
            TypedResults.Ok(await security.GetDashboardAsync(hours ?? 24, ct)))
            .Produces<SecurityDashboardResponse>(StatusCodes.Status200OK);
        group.MapPost("/access-log", async Task<IResult> (AccessLogIngestRequest request, SecurityIngestionService security, CancellationToken ct) =>
        {
            await security.IngestAccessLogAsync(request, ct);
            return TypedResults.Ok(new { accepted = true });
        }).AllowAnonymous();
        group.MapPost("/blocklist/sync", async Task<IResult> (FirewallApplyRequest request, SecurityIngestionService security, CancellationToken ct) =>
        {
            await security.SyncBlocklistToFirewallAsync(request, ct);
            return TypedResults.Ok(new { synced = true });
        });
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
        group.MapPost("/agents", async Task<Ok<CreatePulseAgentResponse>> (CreatePulseAgentRequest request, PulseAgentService pulse, CancellationToken ct) =>
        {
            var created = await pulse.CreateAgentAsync(request, ct);
            return TypedResults.Ok(created);
        });
        group.MapPost("/{agentId:guid}/heartbeat", async Task<IResult> (
            Guid agentId,
            PulseHeartbeatAuthRequest request,
            HttpContext ctx,
            PulseAgentService pulse,
            CancellationToken ct) =>
        {
            var accepted = await pulse.AcceptHeartbeatAsync(
                agentId,
                request,
                ctx.Connection.RemoteIpAddress?.ToString(),
                ct);
            return accepted ? TypedResults.Ok(new { accepted = true }) : TypedResults.Unauthorized();
        }).AllowAnonymous();
        return app;
    }
}

public static class ScriptEndpoints
{
    public static IEndpointRouteBuilder MapScriptEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app.MapGroup("/api/scripts").WithTags("Scripts");
        group.MapGet("/", async (ScriptExecutionService scripts, CancellationToken ct) =>
            TypedResults.Ok(await scripts.ListAsync(ct)));
        group.MapPost("/", async Task<IResult> (CreateScriptRequest request, ScriptExecutionService scripts, CancellationToken ct) =>
        {
            var created = await scripts.CreateAsync(request, ct);
            return TypedResults.Ok(created);
        })
            .Produces<ScriptResponse>(StatusCodes.Status200OK);
        group.MapPost("/{scriptId:guid}/run", async Task<IResult> (
            Guid scriptId,
            RunScriptRequest request,
            ScriptExecutionService scripts,
            CancellationToken ct) =>
        {
            var result = string.IsNullOrWhiteSpace(request.Host)
                ? await scripts.RunWithConnectionAsync(scriptId, ct)
                : await scripts.RunAsync(scriptId, request, ct);
            return TypedResults.Ok(result);
        })
            .Produces<RunScriptResponse>(StatusCodes.Status200OK);
        return app;
    }
}

public static class NotificationEndpoints
{
    public static IEndpointRouteBuilder MapNotificationEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app.MapGroup("/api/settings/notifications").WithTags("Settings");
        group.MapGet("/providers", async (NotificationDispatcher notifications, CancellationToken ct) =>
            TypedResults.Ok(await notifications.ListProvidersAsync(ct)));
        group.MapPost("/providers", async Task<IResult> (CreateNotificationProviderRequest request, NotificationDispatcher notifications, CancellationToken ct) =>
        {
            var created = await notifications.CreateProviderAsync(request, ct);
            return TypedResults.Ok(created);
        })
            .Produces<NotificationProviderResponse>(StatusCodes.Status200OK);
        group.MapPost("/send", async Task<IResult> (SendNotificationRequest request, NotificationDispatcher notifications, CancellationToken ct) =>
        {
            await notifications.SendAsync(request, ct);
            return TypedResults.Ok(new { sent = true });
        });
        return app;
    }
}

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
            TypedResults.Ok(await adguard.CreateConnectionAsync(request, ct)))
            .Produces<AdGuardConnectionResponse>(StatusCodes.Status200OK);
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
            .Produces<AdGuardRewriteResponse>(StatusCodes.Status200OK);
        group.MapPost("/{connectionId:guid}/sync", async Task<IResult> (
            Guid connectionId,
            AdGuardSyncService adguard,
            CancellationToken ct) =>
        {
            await adguard.SyncManagedRewritesAsync(connectionId, ct);
            return TypedResults.Ok(new { synced = true });
        });
        return app;
    }
}

public static class WafEndpoints
{
    public static IEndpointRouteBuilder MapWafEndpoints(this IEndpointRouteBuilder app)
    {
        app.MapGet("/api/waf/{slug}/middleware", (string slug) =>
        {
            var yaml = Hashi.Core.Security.WafMiddlewareRenderer.RenderCorazaMiddleware(slug, Hashi.Core.Security.WafMode.On);
            return TypedResults.Ok(new { slug, yaml });
        }).WithTags("Security");
        return app;
    }
}
