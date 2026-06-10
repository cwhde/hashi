using Hashi.Api.Hosting;
using Hashi.Contracts.Api;
using Hashi.Infrastructure.Platform;
using Microsoft.AspNetCore.Http;
using Microsoft.AspNetCore.Http.HttpResults;
using Microsoft.AspNetCore.Routing;

namespace Hashi.Api.Features.InternalAgentDns;

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
