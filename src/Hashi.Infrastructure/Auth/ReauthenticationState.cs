using System.Collections.Concurrent;
using Microsoft.AspNetCore.Http;

namespace Hashi.Infrastructure.Auth;

public sealed class ReauthenticationState
{
    private static readonly TimeSpan ReauthWindow = TimeSpan.FromMinutes(5);
    private readonly ConcurrentDictionary<string, DateTimeOffset> _recent = new();

    public void MarkRecent(HttpContext context)
    {
        var key = SessionKey(context);
        if (key is null)
        {
            return;
        }

        _recent[key] = DateTimeOffset.UtcNow;
    }

    public bool IsRecent(HttpContext context)
    {
        var key = SessionKey(context);
        if (key is null)
        {
            return false;
        }

        return _recent.TryGetValue(key, out var at)
            && DateTimeOffset.UtcNow - at <= ReauthWindow;
    }

    private static string? SessionKey(HttpContext context)
    {
        var sessionId = context.User.FindFirst(System.Security.Claims.ClaimTypes.NameIdentifier)?.Value
            ?? context.User.FindFirst(System.Security.Claims.ClaimTypes.Name)?.Value;
        if (!string.IsNullOrEmpty(sessionId))
        {
            return sessionId;
        }

        return context.Request.Cookies["hashi.session"];
    }
}
