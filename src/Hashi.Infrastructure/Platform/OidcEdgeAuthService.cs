using Hashi.Contracts.Api;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Microsoft.AspNetCore.Http;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Infrastructure.Platform;

public sealed class OidcEdgeAuthService(HashiDbContext db)
{
    public async Task<IReadOnlyList<OidcProviderEntity>> ListProvidersAsync(CancellationToken cancellationToken = default)
        => await db.OidcProviders.AsNoTracking().Where(x => x.Enabled).ToListAsync(cancellationToken);

    public string BuildLoginRedirectUri(HttpContext context, Guid providerId, string returnUrl)
    {
        var baseUrl = $"{context.Request.Scheme}://{context.Request.Host}";
        return $"{baseUrl}/api/edge-auth/login?providerId={providerId}&returnUrl={Uri.EscapeDataString(returnUrl)}";
    }
}

public sealed record EdgeSessionState(string Subject, DateTimeOffset ExpiresAtUtc);

public static class EdgeSessionStore
{
    private static readonly Dictionary<string, EdgeSessionState> Sessions = new();

    public static void Set(string sessionKey, EdgeSessionState state) => Sessions[sessionKey] = state;

    public static bool TryGet(string sessionKey, out EdgeSessionState? state) => Sessions.TryGetValue(sessionKey, out state);
}
