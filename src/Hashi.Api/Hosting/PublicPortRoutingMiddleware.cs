using Hashi.Infrastructure.Services;
using Microsoft.AspNetCore.Http;

namespace Hashi.Api.Hosting;

public sealed class PublicPortRoutingMiddleware(RequestDelegate next)
{
    public async Task InvokeAsync(HttpContext context, AppSettingsService settings)
    {
        var port = context.Connection.LocalPort;
        var path = context.Request.Path.Value ?? "/";

        if (path.StartsWith("/api/", StringComparison.OrdinalIgnoreCase)
            || path.StartsWith("/openapi/", StringComparison.OrdinalIgnoreCase))
        {
            await next(context);
            return;
        }

        if (port == 8081)
        {
            var appSettings = await settings.GetOrCreateAsync(context.RequestAborted);
            if (!appSettings.PublicDashboardEnabled)
            {
                context.Response.StatusCode = StatusCodes.Status404NotFound;
                return;
            }

            if (path is "/" or "")
            {
                context.Response.Redirect("/dashboard");
                return;
            }
        }

        if (port == 8082)
        {
            var appSettings = await settings.GetOrCreateAsync(context.RequestAborted);
            if (!appSettings.PublicStatusEnabled)
            {
                context.Response.StatusCode = StatusCodes.Status404NotFound;
                return;
            }

            if (path is "/" or "")
            {
                context.Response.Redirect("/status-page");
                return;
            }
        }

        await next(context);
    }
}
