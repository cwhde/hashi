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

        // Bootstrap login and passkey ceremonies establish the session.
        if (context.Request.Path.StartsWithSegments("/api/auth"))
        {
            await next(context);
            return;
        }

        // Unsafe setup mutations during bootstrap (before passkey session + CSRF flow).
        if (context.Request.Path.StartsWithSegments("/api/setup")
            && context.Request.Path.Value?.Contains("/complete", StringComparison.Ordinal) == true)
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
}
