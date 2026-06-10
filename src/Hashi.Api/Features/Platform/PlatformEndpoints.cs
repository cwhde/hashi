using FluentValidation;
using System.Net;
using System.Text.Json;
using Hashi.Api.Hosting;
using Hashi.Contracts.Api;
using Hashi.Core.Traefik;
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
                return TypedResults.Ok(await resources.ToResponseAsync(created, ct));
            }
            catch (InvalidOperationException ex)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(ex.Message));
            }
        });

        group.MapPut("/{id:guid}", async Task<IResult> (Guid id, UpdateResourceRequest request, ResourceService resources, CancellationToken ct) =>
        {
            try
            {
                var updated = await resources.UpdateAsync(id, request, ct);
                return updated is null
                    ? TypedResults.NotFound()
                    : TypedResults.Ok(await resources.ToResponseAsync(updated, ct));
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
        group.MapPut("/hosts/{firewallHostId:guid}", async Task<IResult> (
            Guid firewallHostId,
            UpdateFirewallHostRequest request,
            FirewallApplyService firewall,
            CancellationToken ct) =>
        {
            var host = await firewall.UpdateHostAsync(firewallHostId, request, ct);
            return host is null ? TypedResults.NotFound() : TypedResults.Ok(FirewallApplyService.ToResponse(host));
        })
            .Produces<FirewallHostResponse>(StatusCodes.Status200OK);
        group.MapPost("/hosts/{firewallHostId:guid}/plan", async (
            Guid firewallHostId,
            FirewallApplyService firewall,
            CancellationToken ct) =>
            TypedResults.Ok(await firewall.PlanForHostAsync(firewallHostId, ct)))
            .Produces<FirewallPlanPreviewResponse>(StatusCodes.Status200OK);
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

public static class PublicEndpoints
{
    public static IEndpointRouteBuilder MapPublicEndpoints(this IEndpointRouteBuilder app)
    {
        app.MapGet("/api/public/status", async Task<IResult> (MonitoringService monitoring, CancellationToken ct) =>
        {
            if (!await monitoring.IsPublicStatusEnabledAsync(ct))
            {
                return TypedResults.NotFound();
            }

            return TypedResults.Ok(await monitoring.PublicStatusAsync(ct));
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

public static class EdgeAuthEndpoints
{
    public static IEndpointRouteBuilder MapEdgeAuthEndpoints(this IEndpointRouteBuilder app)
    {
        app.MapGet("/api/edge-auth/forward", async Task<IResult> (
            HttpContext ctx,
            EdgeAuthService edgeAuth,
            SecurityIngestionService security,
            GeoIpLookupService geoIp,
            ForwardedClientContextResolver forwardedContext,
            CancellationToken ct) =>
        {
            var requestContext = forwardedContext.Resolve(ctx);
            var host = requestContext.TrustedProxy
                ? ctx.Request.Headers["X-Forwarded-Host"].FirstOrDefault() ?? ctx.Request.Host.Value ?? string.Empty
                : ctx.Request.Host.Value ?? string.Empty;
            var path = requestContext.TrustedProxy
                ? ctx.Request.Headers["X-Forwarded-Uri"].FirstOrDefault()
                    ?? ctx.Request.Headers["X-Original-URL"].FirstOrDefault()
                    ?? ctx.Request.Path.Value
                    ?? "/"
                : ctx.Request.Path.Value ?? "/";
            var clientIp = requestContext.ClientIp;
            var country = requestContext.TrustedProxy ? ctx.Request.Headers["X-Geo-Country"].FirstOrDefault() : null;
            var region = requestContext.TrustedProxy ? ctx.Request.Headers["X-Geo-Region"].FirstOrDefault() : null;
            var asn = requestContext.TrustedProxy ? ctx.Request.Headers["X-Geo-Asn"].FirstOrDefault() : null;
            var requestId = ctx.Request.Headers["X-Request-Id"].FirstOrDefault()
                ?? ctx.Request.Headers["X-Correlation-Id"].FirstOrDefault()
                ?? ctx.Request.Headers["Request-Id"].FirstOrDefault()
                ?? ctx.TraceIdentifier;
            var userAgent = ctx.Request.Headers["User-Agent"].FirstOrDefault();
            if (country is null && region is null && asn is null)
            {
                var lookup = geoIp.Lookup(clientIp);
                if (lookup is not null)
                {
                    country ??= lookup.CountryCode;
                    region ??= lookup.RegionCode;
                    asn ??= lookup.Asn;
                }
            }

            var mode = ctx.Request.Query["mode"].FirstOrDefault();
            var result = await edgeAuth.EvaluateForwardDecisionAsync(
                new SecurityDecisionRequest(
                    host,
                    path,
                    clientIp,
                    country,
                    region,
                    asn,
                    ctx.Request.Cookies["hashi.edge.session"],
                    mode,
                    requestContext.TrustedProxy,
                    requestContext.Method,
                    ctx.Request.Headers.Accept.FirstOrDefault()),
                ct);

            await security.IngestForwardAuthDecisionAsync(new ForwardAuthDecisionIngestRequest(
                clientIp.ToString(),
                host,
                path,
                result.Decision,
                country,
                asn,
                region,
                requestContext.Method,
                path,
                requestId,
                userAgent), ct);

            return result.ResponseMode switch
            {
                SecurityDecisionResponseModeNames.Allow => TypedResults.StatusCode(StatusCodes.Status204NoContent),
                SecurityDecisionResponseModeNames.Redirect => TypedResults.Redirect(result.RedirectUrl ?? "/api/edge-auth/login"),
                SecurityDecisionResponseModeNames.ApiChallenge => TypedResults.Json(
                    new { challenge_required = true, reason = result.Reason },
                    statusCode: result.StatusCode),
                SecurityDecisionResponseModeNames.RateLimited => TypedResults.Json(
                    new { rate_limited = true, reason = result.Reason },
                    statusCode: StatusCodes.Status429TooManyRequests),
                _ => TypedResults.StatusCode(result.StatusCode),
            };
        }).WithTags("EdgeAuth").AllowAnonymous();

        app.MapGet("/api/edge-auth/login", async Task<IResult> (
            HttpContext ctx,
            Guid? providerId,
            string? returnUrl,
            bool? rememberMe,
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

            var authorizationUrl = await oidc.BuildAuthorizationUrlAsync(ctx, provider.Id, returnUrl ?? "/", rememberMe == true, ct);
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

            try
            {
                var result = await oidc.CompleteCallbackAsync(ctx, providerId, code, state, ct);
                ctx.Response.Cookies.Append("hashi.edge.session", result.SessionKey, result.SessionCookie);
                return TypedResults.Redirect(result.ReturnUrl);
            }
            catch (InvalidOperationException ex)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(ex.Message));
            }
        }).WithTags("EdgeAuth").AllowAnonymous();

        app.MapPost("/api/edge-auth/logout", async Task<IResult> (
            HttpContext ctx,
            OidcEdgeAuthService oidc,
            CancellationToken ct) =>
        {
            var sessionKey = ctx.Request.Cookies["hashi.edge.session"];
            await oidc.ClearSessionAsync(sessionKey, ct);
            if (!string.IsNullOrWhiteSpace(sessionKey))
            {
                ctx.Response.Cookies.Delete("hashi.edge.session");
            }

            return TypedResults.Ok(new { loggedOut = true });
        }).WithTags("EdgeAuth").AllowAnonymous();

        return app;
    }
}

public static class EdgeChallengeEndpoints
{
    public static IEndpointRouteBuilder MapEdgeChallengeEndpoints(this IEndpointRouteBuilder app)
    {
        app.MapGet("/api/edge-challenge/start", async Task<IResult> (
            HttpContext ctx,
            string? returnUrl,
            CaptchaChallengeService captcha,
            ForwardedClientContextResolver forwardedContext,
            CancellationToken ct) =>
        {
            var requestContext = forwardedContext.Resolve(ctx);
            await captcha.RecordChallengePageRequestAsync(requestContext.ClientIp, returnUrl, ct);
            var query = string.IsNullOrWhiteSpace(returnUrl)
                ? string.Empty
                : $"?returnUrl={Uri.EscapeDataString(returnUrl)}";
            return TypedResults.Redirect($"/challenge{query}");
        }).WithTags("EdgeChallenge").AllowAnonymous();

        app.MapGet("/api/edge-challenge/status", async (
            HttpContext ctx,
            string? returnUrl,
            CaptchaChallengeService captcha,
            ForwardedClientContextResolver forwardedContext,
            CancellationToken ct) =>
        {
            var requestContext = forwardedContext.Resolve(ctx);
            return TypedResults.Ok(await captcha.GetChallengeStatusAsync(requestContext.ClientIp, returnUrl, ct));
        })
            .WithTags("EdgeChallenge")
            .AllowAnonymous()
            .Produces<CaptchaChallengeStatusResponse>(StatusCodes.Status200OK);

        app.MapPost("/api/edge-challenge/verify", async Task<IResult> (
            HttpContext ctx,
            CaptchaChallengeVerifyRequest request,
            CaptchaChallengeService captcha,
            ForwardedClientContextResolver forwardedContext,
            CancellationToken ct) =>
        {
            var requestContext = forwardedContext.Resolve(ctx);
            var result = await captcha.VerifyChallengeAsync(requestContext.ClientIp, request, ct);
            return result.Status switch
            {
                "unavailable" => TypedResults.Json(result, statusCode: StatusCodes.Status503ServiceUnavailable),
                "failed" => TypedResults.Json(result, statusCode: StatusCodes.Status403Forbidden),
                _ => TypedResults.Ok(result),
            };
        })
            .WithTags("EdgeChallenge")
            .AllowAnonymous()
            .Produces<CaptchaChallengeVerifyResponse>(StatusCodes.Status200OK)
            .Produces<CaptchaChallengeVerifyResponse>(StatusCodes.Status403Forbidden)
            .Produces<CaptchaChallengeVerifyResponse>(StatusCodes.Status503ServiceUnavailable);

        return app;
    }
}

public static class EdgeSsoAdminEndpoints
{
    public static IEndpointRouteBuilder MapEdgeSsoAdminEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app.MapGroup("/api/settings/edge-sso").WithTags("Settings");
        group.MapGet("/providers", async (OidcProviderAdminService admin, CancellationToken ct) =>
            TypedResults.Ok(await admin.ListProvidersAsync(ct)));
        group.MapPost("/providers", async Task<IResult> (
            CreateOidcProviderRequest request,
            OidcProviderAdminService admin,
            CancellationToken ct) =>
            TypedResults.Ok(await admin.CreateProviderAsync(request, ct)));
        group.MapPut("/providers/{providerId:guid}", async Task<IResult> (
            Guid providerId,
            UpdateOidcProviderRequest request,
            OidcProviderAdminService admin,
            CancellationToken ct) =>
        {
            var updated = await admin.UpdateProviderAsync(providerId, request, ct);
            return updated is null ? TypedResults.NotFound() : TypedResults.Ok(updated);
        });
        group.MapDelete("/providers/{providerId:guid}", async Task<IResult> (
            Guid providerId,
            OidcProviderAdminService admin,
            CancellationToken ct) =>
        {
            var deleted = await admin.DeleteProviderAsync(providerId, ct);
            return deleted ? TypedResults.NoContent() : TypedResults.NotFound();
        });
        group.MapGet("/rules", async (OidcProviderAdminService admin, CancellationToken ct) =>
            TypedResults.Ok(await admin.ListRulesAsync(ct)));
        group.MapPost("/rules", async Task<IResult> (
            CreateEdgeAuthRuleRequest request,
            OidcProviderAdminService admin,
            CancellationToken ct) =>
        {
            try
            {
                return TypedResults.Ok(await admin.CreateRuleAsync(request, ct));
            }
            catch (InvalidOperationException ex)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(ex.Message));
            }
        });
        group.MapPut("/rules/{ruleId:guid}", async Task<IResult> (
            Guid ruleId,
            UpdateEdgeAuthRuleRequest request,
            OidcProviderAdminService admin,
            CancellationToken ct) =>
        {
            try
            {
                var updated = await admin.UpdateRuleAsync(ruleId, request, ct);
                return updated is null ? TypedResults.NotFound() : TypedResults.Ok(updated);
            }
            catch (InvalidOperationException ex)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(ex.Message));
            }
        });
        group.MapDelete("/rules/{ruleId:guid}", async Task<IResult> (
            Guid ruleId,
            OidcProviderAdminService admin,
            CancellationToken ct) =>
        {
            var deleted = await admin.DeleteRuleAsync(ruleId, ct);
            return deleted ? TypedResults.NoContent() : TypedResults.NotFound();
        });
        return app;
    }
}

public static class SecurityEndpoints
{
    public static IEndpointRouteBuilder MapSecurityEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app.MapGroup("/api/security").WithTags("Security");
        group.MapGet("/dashboard", async (
            int? hours,
            string? resource,
            string? traefikHost,
            Guid? firewallHostId,
            SecurityIngestionService security,
            CancellationToken ct) =>
            TypedResults.Ok(await security.GetDashboardAsync(
                hours ?? 24,
                resource,
                traefikHost,
                firewallHostId,
                ct)))
            .Produces<SecurityDashboardResponse>(StatusCodes.Status200OK);
        group.MapGet("/captcha/settings", async (CaptchaChallengeService captcha, CancellationToken ct) =>
            TypedResults.Ok(await captcha.GetSettingsAsync(ct)))
            .Produces<CaptchaSettingsResponse>(StatusCodes.Status200OK);
        group.MapPut("/captcha/settings", async Task<IResult> (
            CaptchaSettingsRequest request,
            CaptchaChallengeService captcha,
            CancellationToken ct) =>
        {
            try
            {
                return TypedResults.Ok(await captcha.UpdateSettingsAsync(request, ct));
            }
            catch (InvalidOperationException ex)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(ex.Message));
            }
        })
            .Produces<CaptchaSettingsResponse>(StatusCodes.Status200OK)
            .Produces<ApiErrorResponse>(StatusCodes.Status400BadRequest);
        group.MapPost("/captcha/test", async Task<IResult> (
            CaptchaTestRequest request,
            CaptchaChallengeService captcha,
            CancellationToken ct) =>
        {
            var result = await captcha.TestAsync(request, ct);
            return result.Status switch
            {
                "unavailable" => TypedResults.Json(result, statusCode: StatusCodes.Status503ServiceUnavailable),
                "failed" => TypedResults.Json(result, statusCode: StatusCodes.Status400BadRequest),
                _ => TypedResults.Ok(result),
            };
        })
            .Produces<CaptchaTestResponse>(StatusCodes.Status200OK)
            .Produces<CaptchaTestResponse>(StatusCodes.Status400BadRequest)
            .Produces<CaptchaTestResponse>(StatusCodes.Status503ServiceUnavailable);
        group.MapPost("/access-log", async Task<IResult> (AccessLogIngestRequest request, SecurityIngestionService security, CancellationToken ct) =>
        {
            await security.IngestAccessLogAsync(request, ct);
            return TypedResults.Ok(new { accepted = true });
        });
        group.MapPost("/waf-events", async Task<IResult> (WafEventIngestRequest request, SecurityIngestionService security, CancellationToken ct) =>
        {
            await security.IngestWafEventAsync(request, ct);
            return TypedResults.Ok(new { accepted = true });
        });
        group.MapPost("/blocklist/sync", async Task<IResult> (SecurityIngestionService security, CancellationToken ct) =>
        {
            var result = await security.SyncBlocklistToAllFirewallsAsync(ct);
            return TypedResults.Ok(result);
        }).Produces<BlocklistSyncResponse>(StatusCodes.Status200OK);
        group.MapGet("/subjects/search", async (
            string? q,
            SecuritySubjectOperationsService subjects,
            CancellationToken ct) =>
            TypedResults.Ok(await subjects.SearchAsync(q, ct)))
            .Produces<SecuritySubjectSearchResponse>(StatusCodes.Status200OK);
        group.MapGet("/subjects/{id:guid}", async Task<IResult> (
            Guid id,
            SecuritySubjectOperationsService subjects,
            CancellationToken ct) =>
        {
            var detail = await subjects.GetDetailAsync(id, ct);
            return detail is null ? TypedResults.NotFound() : TypedResults.Ok(detail);
        })
            .Produces<SecuritySubjectDetailResponse>(StatusCodes.Status200OK);
        group.MapGet("/subjects/{id:guid}/events", async (
            Guid id,
            string? eventType,
            Guid? resourceId,
            int? limit,
            SecuritySubjectOperationsService subjects,
            CancellationToken ct) =>
            TypedResults.Ok(await subjects.ListEventsAsync(id, eventType, resourceId, limit ?? 100, ct)))
            .Produces<IEnumerable<SecurityEventResponse>>(StatusCodes.Status200OK);
        group.MapGet("/subjects/{id:guid}/buckets", async (
            Guid id,
            int? hours,
            SecuritySubjectOperationsService subjects,
            CancellationToken ct) =>
            TypedResults.Ok(await subjects.ListBucketsAsync(id, hours ?? 24, ct)))
            .Produces<IEnumerable<SecurityRequestBucketResponse>>(StatusCodes.Status200OK);
        group.MapGet("/subjects/{id:guid}/effective-decision", async Task<IResult> (
            Guid id,
            SecuritySubjectOperationsService subjects,
            CancellationToken ct) =>
        {
            var decision = await subjects.GetEffectiveDecisionAsync(id, ct);
            return decision is null ? TypedResults.NotFound() : TypedResults.Ok(decision);
        })
            .Produces<SecurityEffectiveDecisionResponse>(StatusCodes.Status200OK);
        group.MapPost("/manual-entries", async Task<IResult> (
            UpsertManualSecurityEntryRequest request,
            SecuritySubjectOperationsService subjects,
            CancellationToken ct) =>
        {
            try
            {
                return TypedResults.Ok(await subjects.CreateManualEntryAsync(request, ct));
            }
            catch (InvalidOperationException ex)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(ex.Message));
            }
        })
            .Produces<ManualSecurityEntryResponse>(StatusCodes.Status200OK)
            .Produces<ApiErrorResponse>(StatusCodes.Status400BadRequest);
        group.MapPatch("/manual-entries/{id:guid}", async Task<IResult> (
            Guid id,
            UpdateManualSecurityEntryRequest request,
            SecuritySubjectOperationsService subjects,
            CancellationToken ct) =>
        {
            try
            {
                var entry = await subjects.UpdateManualEntryAsync(id, request, ct);
                return entry is null ? TypedResults.NotFound() : TypedResults.Ok(entry);
            }
            catch (InvalidOperationException ex)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(ex.Message));
            }
        })
            .Produces<ManualSecurityEntryResponse>(StatusCodes.Status200OK)
            .Produces<ApiErrorResponse>(StatusCodes.Status400BadRequest);
        group.MapDelete("/manual-entries/{id:guid}", async Task<IResult> (
            Guid id,
            SecuritySubjectOperationsService subjects,
            CancellationToken ct) =>
        {
            var deleted = await subjects.DeleteManualEntryAsync(id, ct);
            return deleted ? TypedResults.NoContent() : TypedResults.NotFound();
        });
        group.MapPost("/manual-entries/{id:guid}/expire", async Task<IResult> (
            Guid id,
            SecuritySubjectOperationsService subjects,
            CancellationToken ct) =>
        {
            var entry = await subjects.ExpireManualEntryAsync(id, ct);
            return entry is null ? TypedResults.NotFound() : TypedResults.Ok(entry);
        })
            .Produces<ManualSecurityEntryResponse>(StatusCodes.Status200OK);
        group.MapPost("/blocks", async Task<IResult> (
            CreateSecurityBlockRequest request,
            SecuritySubjectOperationsService subjects,
            CancellationToken ct) =>
        {
            try
            {
                return TypedResults.Ok(await subjects.CreateBlockAsync(request, ct));
            }
            catch (InvalidOperationException ex)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(ex.Message));
            }
        })
            .Produces<SecurityBlockMutationResponse>(StatusCodes.Status200OK)
            .Produces<ApiErrorResponse>(StatusCodes.Status400BadRequest);
        group.MapPatch("/blocks/{id:guid}", async Task<IResult> (
            Guid id,
            UpdateSecurityBlockRequest request,
            SecuritySubjectOperationsService subjects,
            CancellationToken ct) =>
        {
            try
            {
                var entry = await subjects.UpdateBlockAsync(id, request, ct);
                return entry is null ? TypedResults.NotFound() : TypedResults.Ok(entry);
            }
            catch (InvalidOperationException ex)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(ex.Message));
            }
        })
            .Produces<SecurityBlockMutationResponse>(StatusCodes.Status200OK)
            .Produces<ApiErrorResponse>(StatusCodes.Status400BadRequest);
        group.MapPost("/blocks/{id:guid}/extend", async Task<IResult> (
            Guid id,
            SecurityBlockDurationRequest request,
            SecuritySubjectOperationsService subjects,
            CancellationToken ct) =>
        {
            var entry = await subjects.ExtendBlockAsync(id, request.DurationSeconds, ct);
            return entry is null ? TypedResults.NotFound() : TypedResults.Ok(entry);
        })
            .Produces<SecurityBlockMutationResponse>(StatusCodes.Status200OK);
        group.MapPost("/blocks/{id:guid}/shorten", async Task<IResult> (
            Guid id,
            SecurityBlockDurationRequest request,
            SecuritySubjectOperationsService subjects,
            CancellationToken ct) =>
        {
            var entry = await subjects.ShortenBlockAsync(id, request.DurationSeconds, ct);
            return entry is null ? TypedResults.NotFound() : TypedResults.Ok(entry);
        })
            .Produces<SecurityBlockMutationResponse>(StatusCodes.Status200OK);
        group.MapPost("/blocks/{id:guid}/make-permanent", async Task<IResult> (
            Guid id,
            SecuritySubjectOperationsService subjects,
            CancellationToken ct) =>
        {
            var entry = await subjects.MakePermanentAsync(id, ct);
            return entry is null ? TypedResults.NotFound() : TypedResults.Ok(entry);
        })
            .Produces<SecurityBlockMutationResponse>(StatusCodes.Status200OK);
        group.MapPost("/blocks/{id:guid}/expire", async Task<IResult> (
            Guid id,
            SecuritySubjectOperationsService subjects,
            CancellationToken ct) =>
        {
            var entry = await subjects.ExpireBlockAsync(id, ct);
            return entry is null ? TypedResults.NotFound() : TypedResults.Ok(entry);
        })
            .Produces<SecurityBlockMutationResponse>(StatusCodes.Status200OK);
        group.MapPost("/blocks/{id:guid}/preview-firewall-sync", async Task<IResult> (
            Guid id,
            SecuritySubjectOperationsService subjects,
            CancellationToken ct) =>
        {
            try
            {
                var preview = await subjects.PreviewFirewallSyncAsync(id, ct);
                return preview is null ? TypedResults.NotFound() : TypedResults.Ok(preview);
            }
            catch (InvalidOperationException ex)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(ex.Message));
            }
        })
            .Produces<FirewallPlanPreviewResponse>(StatusCodes.Status200OK)
            .Produces<ApiErrorResponse>(StatusCodes.Status400BadRequest);
        group.MapGet("/blocklists", async (BlocklistSourceManagementService blocklists, CancellationToken ct) =>
            TypedResults.Ok(await blocklists.ListAsync(ct)))
            .Produces<IEnumerable<BlocklistSourceResponse>>(StatusCodes.Status200OK);
        group.MapPost("/blocklists", async Task<IResult> (
            UpsertBlocklistSourceRequest request,
            BlocklistSourceManagementService blocklists,
            CancellationToken ct) =>
        {
            try
            {
                return TypedResults.Ok(await blocklists.CreateAsync(request, ct));
            }
            catch (InvalidOperationException ex)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(ex.Message));
            }
        })
            .Produces<BlocklistSourceResponse>(StatusCodes.Status200OK)
            .Produces<ApiErrorResponse>(StatusCodes.Status400BadRequest);
        group.MapGet("/blocklists/{id:guid}", async Task<IResult> (
            Guid id,
            BlocklistSourceManagementService blocklists,
            CancellationToken ct) =>
        {
            var source = await blocklists.GetAsync(id, ct);
            return source is null ? TypedResults.NotFound() : TypedResults.Ok(source);
        })
            .Produces<BlocklistSourceResponse>(StatusCodes.Status200OK);
        group.MapPatch("/blocklists/{id:guid}", async Task<IResult> (
            Guid id,
            UpsertBlocklistSourceRequest request,
            BlocklistSourceManagementService blocklists,
            CancellationToken ct) =>
        {
            try
            {
                var source = await blocklists.UpdateAsync(id, request, ct);
                return source is null ? TypedResults.NotFound() : TypedResults.Ok(source);
            }
            catch (InvalidOperationException ex)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(ex.Message));
            }
        })
            .Produces<BlocklistSourceResponse>(StatusCodes.Status200OK)
            .Produces<ApiErrorResponse>(StatusCodes.Status400BadRequest);
        group.MapDelete("/blocklists/{id:guid}", async Task<IResult> (
            Guid id,
            BlocklistSourceManagementService blocklists,
            CancellationToken ct) =>
            await blocklists.DeleteAsync(id, ct) ? TypedResults.NoContent() : TypedResults.NotFound());
        group.MapPost("/blocklists/{id:guid}/fetch-preview", async Task<IResult> (
            Guid id,
            BlocklistSourceManagementService blocklists,
            CancellationToken ct) =>
        {
            try
            {
                var preview = await blocklists.PreviewAsync(id, ct);
                return preview is null ? TypedResults.NotFound() : TypedResults.Ok(preview);
            }
            catch (InvalidOperationException ex)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(ex.Message));
            }
        })
            .Produces<BlocklistFetchPreviewResponse>(StatusCodes.Status200OK)
            .Produces<ApiErrorResponse>(StatusCodes.Status400BadRequest);
        group.MapPost("/blocklists/{id:guid}/enable", async Task<IResult> (
            Guid id,
            BlocklistSourceManagementService blocklists,
            CancellationToken ct) =>
        {
            var result = await blocklists.EnableAsync(id, ct);
            return result is null ? TypedResults.NotFound() : TypedResults.Ok(result);
        })
            .Produces<BlocklistSourceMutationResponse>(StatusCodes.Status200OK);
        group.MapPost("/blocklists/{id:guid}/disable", async Task<IResult> (
            Guid id,
            BlocklistSourceManagementService blocklists,
            CancellationToken ct) =>
        {
            var result = await blocklists.DisableAsync(id, ct);
            return result is null ? TypedResults.NotFound() : TypedResults.Ok(result);
        })
            .Produces<BlocklistSourceMutationResponse>(StatusCodes.Status200OK);
        group.MapPost("/blocklists/{id:guid}/refresh", async Task<IResult> (
            Guid id,
            BlocklistSourceManagementService blocklists,
            CancellationToken ct) =>
        {
            var result = await blocklists.RefreshAsync(id, ct);
            return result is null ? TypedResults.NotFound() : TypedResults.Ok(result);
        })
            .Produces<BlocklistSourceMutationResponse>(StatusCodes.Status200OK);
        group.MapGet("/blocklists/{id:guid}/runs", async (
            Guid id,
            BlocklistSourceManagementService blocklists,
            CancellationToken ct) =>
            TypedResults.Ok(await blocklists.ListRunsAsync(id, ct)))
            .Produces<IEnumerable<BlocklistFetchRunResponse>>(StatusCodes.Status200OK);
        group.MapGet("/blocklists/{id:guid}/entries", async (
            Guid id,
            BlocklistSourceManagementService blocklists,
            CancellationToken ct) =>
            TypedResults.Ok(await blocklists.ListEntriesAsync(id, ct)))
            .Produces<IEnumerable<BlocklistEntryResponse>>(StatusCodes.Status200OK);
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
            return TypedResults.Ok(agents.Select(PulseAgentService.ToResponse));
        });
        group.MapGet("/agents/{agentId:guid}/resolved-targets", async Task<IResult> (
            Guid agentId,
            HashiDbContext db,
            ConnectionTargetResolver targets,
            CancellationToken ct) =>
        {
            var agent = await db.PulseAgents.AsNoTracking().SingleOrDefaultAsync(x => x.Id == agentId, ct);
            if (agent is null)
            {
                return TypedResults.NotFound();
            }

            var target = new ConnectionTargetEntity
            {
                OwnerType = "pulse_agent_preview",
                OwnerId = agentId,
                TargetMode = ConnectionTargetModeNames.PulseAgent,
                PulseAgentId = agentId,
                PulseIpMode = PulseTargetIpModeNames.Selected,
                PrivateCandidateSelector = PulsePrivateCandidateSelectorNames.Selected,
                Scheme = "http",
                Port = 80,
            };
            var resolved = await targets.ResolveAsync(target, persistSnapshot: false, cancellationToken: ct);
            return TypedResults.Ok(new PulseResolvedTargetResponse(
                agent.Id,
                agent.Name,
                PulseTargetIpModeNames.Selected,
                agent.LastSelectedIp,
                agent.LastPublicIp,
                DeserializeStringList(agent.LastPrivateIpv4CandidatesJson),
                DeserializeStringList(agent.LastPrivateIpv6CandidatesJson),
                agent.LastSeenAtUtc,
                resolved.Status,
                resolved.ResolvedIp,
                resolved.Error));
        })
            .Produces<PulseResolvedTargetResponse>(StatusCodes.Status200OK);
        group.MapPost("/agents", async Task<Ok<CreatePulseAgentResponse>> (CreatePulseAgentRequest request, PulseAgentService pulse, CancellationToken ct) =>
        {
            var created = await pulse.CreateAgentAsync(request, ct);
            return TypedResults.Ok(created);
        });
        group.MapPost("/agents/{agentId:guid}/revoke", async Task<IResult> (Guid agentId, PulseAgentService pulse, CancellationToken ct) =>
        {
            var revoked = await pulse.RevokeAgentAsync(agentId, ct);
            return revoked ? TypedResults.Ok(new { revoked = true }) : TypedResults.NotFound();
        });
        group.MapPost("/agents/{agentId:guid}/rotate-token", async Task<IResult> (Guid agentId, PulseAgentService pulse, CancellationToken ct) =>
        {
            var rotated = await pulse.RotateTokenAsync(agentId, ct);
            return rotated is null ? TypedResults.NotFound() : TypedResults.Ok(rotated);
        })
            .Produces<RotatePulseAgentTokenResponse>(StatusCodes.Status200OK);
        group.MapGet("/agents/{agentId:guid}/install", (HttpContext ctx, Guid agentId) =>
        {
            if (ctx.Request.Query.ContainsKey("token"))
            {
                return Results.BadRequest(new ApiErrorResponse("Pulse tokens must not be sent in URLs."));
            }

            var apiBase = $"{ctx.Request.Scheme}://{ctx.Request.Host}";
            return TypedResults.Ok(PulseInstallRenderer.Render(apiBase, agentId));
        })
            .Produces<PulseInstallResponse>(StatusCodes.Status200OK);
        group.MapGet("/install/linux.sh", () =>
        {
            var candidates = new[]
            {
                Path.Combine(AppContext.BaseDirectory, "content", "pulse", "install.sh"),
                Path.GetFullPath(Path.Combine(AppContext.BaseDirectory, "..", "..", "..", "..", "agents", "pulse", "install.sh")),
                Path.GetFullPath(Path.Combine(Directory.GetCurrentDirectory(), "agents", "pulse", "install.sh")),
            };
            var path = candidates.FirstOrDefault(File.Exists);
            if (path is null)
            {
                return Results.NotFound();
            }

            return Results.Text(File.ReadAllText(path), "text/x-shellscript");
        }).AllowAnonymous();
        group.MapPost("/{agentId:guid}/heartbeat", async Task<IResult> (
            Guid agentId,
            PulseHeartbeatAuthRequest request,
            HttpContext ctx,
            PulseAgentService pulse,
            CancellationToken ct) =>
        {
            var result = await pulse.AcceptHeartbeatAsync(
                agentId,
                request,
                ctx.Connection.RemoteIpAddress?.ToString(),
                ct);
            return result switch
            {
                PulseHeartbeatAcceptResult.Accepted => TypedResults.Ok(new { accepted = true }),
                PulseHeartbeatAcceptResult.InvalidTimestamp => TypedResults.BadRequest(new ApiErrorResponse("Heartbeat timestamp is outside the accepted clock skew.")),
                _ => TypedResults.Unauthorized(),
            };
        }).AllowAnonymous();
        return app;
    }

    private static IReadOnlyList<string> DeserializeStringList(string? json)
    {
        if (string.IsNullOrWhiteSpace(json))
        {
            return [];
        }

        try
        {
            return JsonSerializer.Deserialize<IReadOnlyList<string>>(json) ?? [];
        }
        catch (JsonException)
        {
            return [];
        }
    }
}

public static class InternalAgentDnsEndpoints
{
    public static IEndpointRouteBuilder MapInternalAgentDnsEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app.MapGroup("/api/settings/internal-agent-dns").WithTags("Settings");
        group.MapGet("/", async (InternalAgentDnsSettingsService settings, CancellationToken ct) =>
            TypedResults.Ok(await settings.GetAsync(ct)))
            .Produces<InternalAgentDnsSettingsResponse>(StatusCodes.Status200OK);
        group.MapPut("/", async Task<IResult> (
            InternalAgentDnsSettingsRequest request,
            InternalAgentDnsSettingsService settings,
            CancellationToken ct) =>
        {
            try
            {
                return TypedResults.Ok(await settings.UpdateAsync(request, ct));
            }
            catch (InvalidOperationException ex)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(ex.Message));
            }
        })
            .Produces<InternalAgentDnsSettingsResponse>(StatusCodes.Status200OK);
        group.MapPost("/preview-sync", async Task<IResult> (
            InternalAgentDnsSettingsService settings,
            CancellationToken ct) =>
        {
            try
            {
                return TypedResults.Ok(await settings.PreviewSyncAsync(ct));
            }
            catch (InvalidOperationException ex)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(ex.Message));
            }
        })
            .Produces<AdGuardRewritePlanResponse>(StatusCodes.Status200OK);
        group.MapPost("/apply-sync", async Task<IResult> (
            AdGuardRewriteApplyRequest request,
            InternalAgentDnsSettingsService settings,
            CancellationToken ct) =>
        {
            try
            {
                return TypedResults.Ok(await settings.ApplySyncAsync(request, ct));
            }
            catch (InvalidOperationException ex)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(ex.Message));
            }
        })
            .Produces<AdGuardRewriteApplyResponse>(StatusCodes.Status200OK);
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
        group.MapPut("/providers/{providerId:guid}", async Task<IResult> (
            Guid providerId,
            UpdateNotificationProviderRequest request,
            NotificationDispatcher notifications,
            CancellationToken ct) =>
        {
            var updated = await notifications.UpdateProviderAsync(providerId, request, ct);
            return updated is null ? TypedResults.NotFound() : TypedResults.Ok(updated);
        })
            .Produces<NotificationProviderResponse>(StatusCodes.Status200OK);
        group.MapDelete("/providers/{providerId:guid}", async Task<IResult> (
            Guid providerId,
            NotificationDispatcher notifications,
            CancellationToken ct) =>
        {
            var deleted = await notifications.DeleteProviderAsync(providerId, ct);
            return deleted ? TypedResults.NoContent() : TypedResults.NotFound();
        });
        group.MapPost("/providers/{providerId:guid}/test", async Task<IResult> (
            Guid providerId,
            NotificationTestRequest request,
            NotificationDispatcher notifications,
            CancellationToken ct) =>
            TypedResults.Ok(await notifications.TestProviderAsync(providerId, request, ct)))
            .Produces<NotificationTestResponse>(StatusCodes.Status200OK);
        group.MapPost("/telegram/discover-chat", async (
            TelegramChatDiscoveryRequest request,
            NotificationDispatcher notifications,
            CancellationToken ct) =>
            TypedResults.Ok(await notifications.DiscoverTelegramChatAsync(request.BotToken, ct)))
            .Produces<TelegramChatDiscoveryResponse>(StatusCodes.Status200OK);
        group.MapGet("/routes", async (NotificationDispatcher notifications, CancellationToken ct) =>
            TypedResults.Ok(await notifications.ListRoutesAsync(ct)));
        group.MapPost("/routes", async Task<IResult> (CreateNotificationRouteRequest request, NotificationDispatcher notifications, CancellationToken ct) =>
        {
            try
            {
                var created = await notifications.CreateRouteAsync(request, ct);
                return TypedResults.Ok(created);
            }
            catch (InvalidOperationException ex)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(ex.Message));
            }
        })
            .Produces<NotificationRouteResponse>(StatusCodes.Status200OK)
            .Produces<ApiErrorResponse>(StatusCodes.Status400BadRequest);
        group.MapPut("/routes/{routeId:guid}", async Task<IResult> (
            Guid routeId,
            UpdateNotificationRouteRequest request,
            NotificationDispatcher notifications,
            CancellationToken ct) =>
        {
            try
            {
                var updated = await notifications.UpdateRouteAsync(routeId, request, ct);
                return updated is null ? TypedResults.NotFound() : TypedResults.Ok(updated);
            }
            catch (InvalidOperationException ex)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(ex.Message));
            }
        })
            .Produces<NotificationRouteResponse>(StatusCodes.Status200OK)
            .Produces<ApiErrorResponse>(StatusCodes.Status400BadRequest);
        group.MapDelete("/routes/{routeId:guid}", async Task<IResult> (
            Guid routeId,
            NotificationDispatcher notifications,
            CancellationToken ct) =>
        {
            var deleted = await notifications.DeleteRouteAsync(routeId, ct);
            return deleted ? TypedResults.NoContent() : TypedResults.NotFound();
        });
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
