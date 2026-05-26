using Microsoft.AspNetCore.Antiforgery;
using Microsoft.AspNetCore.Http;

namespace Hashi.Api.Hosting;

/// <summary>
/// Validates CSRF token on unsafe HTTP methods for authenticated admin API calls (spec §9).
/// </summary>
public sealed class AdminCsrfMiddleware(RequestDelegate next)
{
    private static readonly HashSet<string> UnsafeMethods = new(StringComparer.OrdinalIgnoreCase)
    {
        HttpMethods.Post,
        HttpMethods.Put,
        HttpMethods.Patch,
        HttpMethods.Delete,
    };

    public async Task InvokeAsync(HttpContext context, IAntiforgery antiforgery)
    {
        if (!context.Request.Path.StartsWithSegments("/api")
            || !UnsafeMethods.Contains(context.Request.Method)
            || context.User.Identity?.IsAuthenticated != true)
        {
            await next(context);
            return;
        }

        // These endpoints establish auth state before a trusted CSRF token can exist.
        if (IsCsrfExemptEndpoint(context.Request.Path, context.Request.Method))
        {
            await next(context);
            return;
        }

        try
        {
            await antiforgery.ValidateRequestAsync(context);
        }
        catch (AntiforgeryValidationException)
        {
            context.Response.StatusCode = StatusCodes.Status403Forbidden;
            return;
        }

        await next(context);
    }

    public static bool IsCsrfExemptEndpoint(PathString path, string method)
    {
        var value = path.Value ?? string.Empty;
        return IsEndpoint(value, method, "/api/auth/bootstrap/login", HttpMethods.Post)
               || IsEndpoint(value, method, "/api/auth/passkeys/login/begin", HttpMethods.Post)
               || IsEndpoint(value, method, "/api/auth/passkeys/login/complete", HttpMethods.Post);
    }

    private static bool IsEndpoint(string actualPath, string actualMethod, string expectedPath, string expectedMethod)
        => string.Equals(actualPath, expectedPath, StringComparison.OrdinalIgnoreCase)
           && string.Equals(actualMethod, expectedMethod, StringComparison.OrdinalIgnoreCase);
}
