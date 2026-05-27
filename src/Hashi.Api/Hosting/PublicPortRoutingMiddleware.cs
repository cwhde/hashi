using Hashi.Infrastructure.Services;
using Microsoft.AspNetCore.Http;

namespace Hashi.Api.Hosting;

/// <summary>
/// Routes dedicated public ports (8081 dashboard, 8082 status) to root-only SPA views.
/// Admin API and OpenAPI are only available on the admin port (8080).
/// </summary>
public sealed class PublicPortRoutingMiddleware(RequestDelegate next)
{
    public async Task InvokeAsync(HttpContext context, AppSettingsService settings)
    {
        var port = context.Connection.LocalPort;
        var path = context.Request.Path.Value ?? "/";

        if (port is HashiPorts.PublicDashboard or HashiPorts.PublicStatus && IsBlockedPublicPortPath(path, port))
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

            if (IsDashboardPublicApiPath(path))
            {
                await next(context);
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

            if (IsStatusPublicApiPath(path))
            {
                await next(context);
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

    private static bool IsBlockedPublicPortPath(string path, int port)
    {
        if (path.StartsWith("/openapi/", StringComparison.OrdinalIgnoreCase))
        {
            return true;
        }

        if (!path.StartsWith("/api/", StringComparison.OrdinalIgnoreCase))
        {
            return false;
        }

        return port switch
        {
            HashiPorts.PublicDashboard => !IsDashboardPublicApiPath(path),
            HashiPorts.PublicStatus => !IsStatusPublicApiPath(path),
            _ => true,
        };
    }

    private static bool IsDashboardPublicApiPath(string path)
        => path.Equals("/api/public/apps", StringComparison.OrdinalIgnoreCase);

    private static bool IsStatusPublicApiPath(string path)
        => path.Equals("/api/public/status", StringComparison.OrdinalIgnoreCase)
            || path.Equals("/api/public/status/summary", StringComparison.OrdinalIgnoreCase);

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
