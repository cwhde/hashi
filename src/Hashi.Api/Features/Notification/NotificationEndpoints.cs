using Hashi.Api.Hosting;
using Hashi.Contracts.Api;
using Hashi.Infrastructure.Notifications;
using Microsoft.AspNetCore.Http;
using Microsoft.AspNetCore.Http.HttpResults;
using Microsoft.AspNetCore.Routing;

namespace Hashi.Api.Features.Notification;

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
