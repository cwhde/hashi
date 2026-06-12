using Hashi.Contracts.Api;
using Hashi.Infrastructure.Platform;
using Hashi.Infrastructure.Services;

namespace Hashi.Api.Features.Setup;

internal static class SettingsGeoIpEndpoints
{
    public static void Map(RouteGroupBuilder group)
    {
        group.MapGet("/geoip", async (GeoIpSettingsService geoIp, CancellationToken ct) =>
            TypedResults.Ok(await geoIp.GetAsync(ct)))
            .Produces<GeoIpSettingsResponse>(StatusCodes.Status200OK);

        group.MapPut("/geoip", async Task<IResult> (
            GeoIpSettingsRequest request,
            GeoIpSettingsService geoIp,
            AuditService audit,
            CancellationToken ct) =>
        {
            try
            {
                var response = await geoIp.UpdateAsync(request, ct);
                await audit.WriteAsync("settings", "geoip_updated", subjectType: "app_settings", cancellationToken: ct);
                return TypedResults.Ok(response);
            }
            catch (InvalidOperationException ex)
            {
                return TypedResults.BadRequest(new ApiErrorResponse(ex.Message));
            }
        })
            .Produces<GeoIpSettingsResponse>(StatusCodes.Status200OK)
            .Produces<ApiErrorResponse>(StatusCodes.Status400BadRequest);

        group.MapPost("/geoip/update", async Task<IResult> (
            GeoIpUpdateService updater,
            BackgroundJobService jobs,
            AuditService audit,
            CancellationToken ct) =>
        {
            await jobs.BeginRunAsync(BackgroundJobKeys.GeoIpUpdate, ct);
            var result = await updater.RunUpdateAsync(ct);
            await jobs.CompleteRunAsync(
                BackgroundJobKeys.GeoIpUpdate,
                result.Succeeded,
                result.Message,
                result.Succeeded ? null : result.Message,
                259200,
                ct);
            await audit.WriteAsync(
                "settings",
                "geoip_update_requested",
                outcome: result.Succeeded ? "success" : "failure",
                subjectType: "app_settings",
                cancellationToken: ct);
            return result.Succeeded ? TypedResults.Ok(result) : TypedResults.BadRequest(result);
        })
            .Produces<GeoIpUpdateResponse>(StatusCodes.Status200OK)
            .Produces<GeoIpUpdateResponse>(StatusCodes.Status400BadRequest);
    }
}
