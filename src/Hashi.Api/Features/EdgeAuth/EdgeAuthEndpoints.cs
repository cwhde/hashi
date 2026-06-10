using Hashi.Api.Hosting;
using Hashi.Contracts.Api;
using Hashi.Infrastructure.Platform;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Microsoft.AspNetCore.Http;
using Microsoft.AspNetCore.Routing;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Api.Features.EdgeAuth;

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
            SecurityDecisionResult result;
            try
            {
                result = await edgeAuth.EvaluateForwardDecisionAsync(
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
            }
            catch (Exception)
            {
                result = SecurityDecisionResult.Create(
                    SecurityDecisionActionNames.AllowUpstream,
                    SecurityDecisionResponseModeNames.Allow,
                    StatusCodes.Status204NoContent,
                    null,
                    "allow",
                    "fail_open_on_error",
                    null,
                    SecuritySubjectNormalizer.NormalizeIp(clientIp),
                    [new SecurityDecisionExplanation("error_handling", "fail_open", "Decision service threw an exception; failing open per policy.")]);
            }

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
            HashiDbContext db,
            CancellationToken ct) =>
        {
            var providers = await oidc.ListProvidersAsync(ct);
            OidcProviderEntity? provider = null;

            if (providerId is Guid id)
            {
                provider = providers.FirstOrDefault(x => x.Id == id);
            }
            else
            {
                if (!string.IsNullOrEmpty(returnUrl) && Uri.TryCreate(returnUrl, UriKind.Absolute, out var uri))
                {
                    var host = uri.Host.ToLowerInvariant();
                    var resource = await db.Resources.AsNoTracking()
                        .FirstOrDefaultAsync(x => x.Enabled && x.Domain != null && x.Domain.ToLower() == host, ct);
                    if (resource?.OidcProviderId is not null)
                    {
                        provider = providers.FirstOrDefault(x => x.Id == resource.OidcProviderId.Value);
                    }
                }

                provider ??= providers.FirstOrDefault(x => x.IsDefault);
                provider ??= providers.FirstOrDefault();
            }

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
