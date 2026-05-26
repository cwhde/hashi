using Hashi.Infrastructure.Services;
using Microsoft.AspNetCore.Http;

namespace Hashi.Api.Hosting;

/// <summary>
/// Routes dedicated public ports (8081 dashboard, 8082 status) to root-only SPA views.
/// API and OpenAPI are only available on the admin port (8080).
/// </summary>
public sealed class PublicPortRoutingMiddleware(RequestDelegate next)
{
    public async Task InvokeAsync(HttpContext context, AppSettingsService settings)
    {
        var port = context.Connection.LocalPort;
        var path = context.Request.Path.Value ?? "/";

        if (port is HashiPorts.PublicDashboard or HashiPorts.PublicStatus && IsApiOrOpenApi(path))
        {
            context.Response.StatusCode = StatusCodes.Status404NotFound;
            return;
        }

        if (port == HashiPorts.PublicDashboard)
        {
            var appSettings = await settings.GetOrCreateAsync(context.RequestAborted);
            if (!appSettings.PublicDashboardEnabled)
            {
                context.Response.StatusCode = StatusCodes.Status404NotFound;
                return;
            }

            if (IsLegacyPublicSubpath(path, "/dashboard"))
            {
                context.Response.Redirect("/");
                return;
            }

            if (!IsRootOrStaticAsset(path))
            {
                context.Response.StatusCode = StatusCodes.Status404NotFound;
                return;
            }
        }

        if (port == HashiPorts.PublicStatus)
        {
            var appSettings = await settings.GetOrCreateAsync(context.RequestAborted);
            if (!appSettings.PublicStatusEnabled)
            {
                context.Response.StatusCode = StatusCodes.Status404NotFound;
                return;
            }

            if (IsLegacyPublicSubpath(path, "/status-page") || IsLegacyPublicSubpath(path, "/status"))
            {
                context.Response.Redirect("/");
                return;
            }

            if (!IsRootOrStaticAsset(path))
            {
                context.Response.StatusCode = StatusCodes.Status404NotFound;
                return;
            }
        }

        await next(context);
    }

    private static bool IsApiOrOpenApi(string path)
        => path.StartsWith("/api/", StringComparison.OrdinalIgnoreCase)
            || path.StartsWith("/openapi/", StringComparison.OrdinalIgnoreCase);

    private static bool IsLegacyPublicSubpath(string path, string prefix)
        => path.Equals(prefix, StringComparison.OrdinalIgnoreCase)
            || path.StartsWith(prefix + "/", StringComparison.OrdinalIgnoreCase);

    private static bool IsRootOrStaticAsset(string path)
    {
        if (path is "/" or "")
        {
            return true;
        }

        if (path.StartsWith("/_app/", StringComparison.OrdinalIgnoreCase))
        {
            return true;
        }

        return path.Equals("/favicon.ico", StringComparison.OrdinalIgnoreCase)
            || path.Equals("/robots.txt", StringComparison.OrdinalIgnoreCase);
    }
}
