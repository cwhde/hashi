using Hashi.Core.Auth;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Services;
using Microsoft.AspNetCore.Http;
using System.Security.Claims;

namespace Hashi.Api.Hosting;

/// <summary>
/// Requires authenticated admin session for protected /api routes (spec §9).
/// </summary>
public sealed class AdminApiAuthMiddleware(RequestDelegate next)
{
    private static readonly PathString[] AlwaysPublicPathPrefixes =
    [
        new("/api/edge-auth"),
        new("/api/edge-challenge"),
        new("/api/public"),
    ];

    public async Task InvokeAsync(
        HttpContext context,
        SetupStateService setupState,
        ReauthenticationState reauth)
    {
        var path = context.Request.Path;
        if (!path.StartsWithSegments("/api"))
        {
            await next(context);
            return;
        }

        if (IsPublicEndpoint(path, context.Request.Method) || IsPulseHeartbeat(path))
        {
            await next(context);
            return;
        }

        var setup = await setupState.GetOrCreateAsync(context.RequestAborted);
        if (context.User.Identity?.IsAuthenticated != true)
        {
            context.Response.StatusCode = StatusCodes.Status401Unauthorized;
            await context.Response.WriteAsJsonAsync(new { error = "Authentication required." });
            return;
        }

        var authMethod = context.User.FindFirstValue(AdminClaimTypes.AuthMethod);
        if (setup.IsComplete && string.Equals(authMethod, AdminAuthMethods.Bootstrap, StringComparison.Ordinal))
        {
            context.Response.StatusCode = StatusCodes.Status403Forbidden;
            await context.Response.WriteAsJsonAsync(new { error = "Bootstrap credentials are no longer valid after setup." });
            return;
        }

        if (RequiresReauthentication(path, context.Request.Method) && !reauth.IsRecent(context))
        {
            context.Response.StatusCode = StatusCodes.Status403Forbidden;
            await context.Response.WriteAsJsonAsync(new { error = "Recent reauthentication required.", code = "reauth_required" });
            return;
        }

        await next(context);
    }

    public static bool RequiresReauthentication(PathString path, string method)
    {
        var value = path.Value ?? string.Empty;
        if (value.StartsWith("/api/vault/secrets/", StringComparison.OrdinalIgnoreCase)
            && value.EndsWith("/reveal", StringComparison.OrdinalIgnoreCase))
        {
            return true;
        }

        if (!IsUnsafeMethod(method))
        {
            return false;
        }

        if (value.StartsWith("/api/vault/secrets/", StringComparison.OrdinalIgnoreCase))
        {
            return true;
        }

        if (value.StartsWith("/api/vault/lock", StringComparison.OrdinalIgnoreCase))
        {
            return true;
        }

        if (value.StartsWith("/api/scripts", StringComparison.OrdinalIgnoreCase))
        {
            return true;
        }

        if (value.StartsWith("/api/connections", StringComparison.OrdinalIgnoreCase)
            && method.Equals(HttpMethods.Post, StringComparison.OrdinalIgnoreCase))
        {
            return true;
        }

        if (value.StartsWith("/api/dns/connections", StringComparison.OrdinalIgnoreCase)
            && method.Equals(HttpMethods.Post, StringComparison.OrdinalIgnoreCase))
        {
            return true;
        }

        if (value.StartsWith("/api/dns/records", StringComparison.OrdinalIgnoreCase))
        {
            return true;
        }

        if (value.StartsWith("/api/settings", StringComparison.OrdinalIgnoreCase))
        {
            return true;
        }

        if (value.StartsWith("/api/pulse/agents", StringComparison.OrdinalIgnoreCase)
            && method.Equals(HttpMethods.Post, StringComparison.OrdinalIgnoreCase))
        {
            return true;
        }

        if (value.StartsWith("/api/notifications", StringComparison.OrdinalIgnoreCase))
        {
            return true;
        }

        if (value.StartsWith("/api/adguard", StringComparison.OrdinalIgnoreCase))
        {
            return true;
        }

        if (value.StartsWith("/api/security/blocklist", StringComparison.OrdinalIgnoreCase)
            || value.StartsWith("/api/security/blocklists", StringComparison.OrdinalIgnoreCase))
        {
            return true;
        }

        if (value.StartsWith("/api/security/manual-entries", StringComparison.OrdinalIgnoreCase)
            || value.StartsWith("/api/security/blocks", StringComparison.OrdinalIgnoreCase)
            || value.StartsWith("/api/security/captcha", StringComparison.OrdinalIgnoreCase))
        {
            return true;
        }

        if (value.Contains("/import/apply", StringComparison.OrdinalIgnoreCase)
            || value.Contains("/sync/apply", StringComparison.OrdinalIgnoreCase)
            || value.Contains("/prune", StringComparison.OrdinalIgnoreCase))
        {
            return true;
        }

        if (value.StartsWith("/api/resources/", StringComparison.OrdinalIgnoreCase)
            && method.Equals(HttpMethods.Delete, StringComparison.OrdinalIgnoreCase))
        {
            return true;
        }

        if (value.StartsWith("/api/firewall/", StringComparison.OrdinalIgnoreCase)
            && (value.Contains("/apply", StringComparison.OrdinalIgnoreCase)
                || value.Contains("/rollback", StringComparison.OrdinalIgnoreCase)))
        {
            return true;
        }

        if (value.StartsWith("/api/traefik/apply", StringComparison.OrdinalIgnoreCase))
        {
            return true;
        }

        if (value.StartsWith("/api/sync/", StringComparison.OrdinalIgnoreCase)
            && value.EndsWith("/apply", StringComparison.OrdinalIgnoreCase))
        {
            return true;
        }

        return false;
    }

    public static bool IsPublicEndpoint(PathString path, string method)
    {
        if (IsPublicExactEndpoint(path, method))
        {
            return true;
        }

        foreach (var prefix in AlwaysPublicPathPrefixes)
        {
            if (path.StartsWithSegments(prefix))
            {
                return true;
            }
        }

        return false;
    }

    private static bool IsUnsafeMethod(string method) =>
        method.Equals(HttpMethods.Post, StringComparison.OrdinalIgnoreCase)
        || method.Equals(HttpMethods.Put, StringComparison.OrdinalIgnoreCase)
        || method.Equals(HttpMethods.Patch, StringComparison.OrdinalIgnoreCase)
        || method.Equals(HttpMethods.Delete, StringComparison.OrdinalIgnoreCase);

    private static bool IsPublicExactEndpoint(PathString path, string method)
    {
        var value = path.Value ?? string.Empty;

        return IsEndpoint(value, method, "/api/health", HttpMethods.Get)
               || IsEndpoint(value, method, "/api/setup/status", HttpMethods.Get)
               || IsEndpoint(value, method, "/api/setup/bootstrap-allowed", HttpMethods.Get)
               || IsEndpoint(value, method, "/api/auth/csrf", HttpMethods.Get)
               || IsEndpoint(value, method, "/api/auth/bootstrap/login", HttpMethods.Post)
               || IsEndpoint(value, method, "/api/auth/passkeys/login/begin", HttpMethods.Post)
               || IsEndpoint(value, method, "/api/auth/passkeys/login/complete", HttpMethods.Post)
               || (value.StartsWith("/api/error/", StringComparison.OrdinalIgnoreCase) && string.Equals(method, HttpMethods.Get, StringComparison.OrdinalIgnoreCase));
    }

    private static bool IsEndpoint(string actualPath, string actualMethod, string expectedPath, string expectedMethod)
        => string.Equals(actualPath, expectedPath, StringComparison.OrdinalIgnoreCase)
           && string.Equals(actualMethod, expectedMethod, StringComparison.OrdinalIgnoreCase);

    private static bool IsPulseHeartbeat(PathString path)
        => path.StartsWithSegments("/api/pulse")
           && path.Value?.EndsWith("/heartbeat", StringComparison.OrdinalIgnoreCase) == true;

}
