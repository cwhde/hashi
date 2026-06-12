using Hashi.Api.Hosting;
using Hashi.Contracts.Api;
using Hashi.Infrastructure.Platform;
using Microsoft.AspNetCore.Http;
using Microsoft.AspNetCore.Routing;

namespace Hashi.Api.Features.EdgeChallenge;

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
