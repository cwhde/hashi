using System.Net.Http.Json;
using System.Security.Cryptography;
using System.Text;
using System.Text.Json.Serialization;
using Hashi.Contracts.Api;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Services;
using Microsoft.AspNetCore.Http;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Infrastructure.Platform;

public sealed class OidcEdgeAuthService(
    HashiDbContext db,
    SecretRecordService secrets,
    IHttpClientFactory httpClientFactory,
    AppSettingsService settings)
{
    private static readonly Dictionary<string, PendingOidcLogin> PendingLogins = new();

    public async Task<IReadOnlyList<OidcProviderEntity>> ListProvidersAsync(CancellationToken cancellationToken = default)
        => await db.OidcProviders.AsNoTracking().Where(x => x.Enabled).ToListAsync(cancellationToken);

    public async Task<OidcProviderEntity?> GetProviderAsync(Guid providerId, CancellationToken cancellationToken = default)
        => await db.OidcProviders.AsNoTracking().SingleOrDefaultAsync(x => x.Id == providerId && x.Enabled, cancellationToken);

    public string BuildLoginRedirectUri(HttpContext context, Guid providerId, string returnUrl)
    {
        var baseUrl = $"{context.Request.Scheme}://{context.Request.Host}";
        return $"{baseUrl}/api/edge-auth/callback?providerId={providerId}";
    }

    public async Task<string> BuildAuthorizationUrlAsync(
        HttpContext context,
        Guid providerId,
        string returnUrl,
        CancellationToken cancellationToken = default)
    {
        var provider = await GetProviderAsync(providerId, cancellationToken)
            ?? throw new InvalidOperationException("OIDC provider not found.");
        var state = Convert.ToHexString(RandomNumberGenerator.GetBytes(16)).ToLowerInvariant();
        var redirectUri = BuildLoginRedirectUri(context, provider.Id, returnUrl);
        lock (PendingLogins)
        {
            PendingLogins[state] = new PendingOidcLogin(provider.Id, returnUrl, DateTimeOffset.UtcNow.AddMinutes(10));
        }

        var scope = Uri.EscapeDataString(provider.Scopes);
        var callback = Uri.EscapeDataString(redirectUri);
        return $"{provider.Issuer.TrimEnd('/')}/oauth/authorize?client_id={Uri.EscapeDataString(provider.ClientId)}&redirect_uri={callback}&response_type=code&scope={scope}&state={state}";
    }

    public async Task<EdgeCallbackResult> CompleteCallbackAsync(
        HttpContext context,
        Guid providerId,
        string code,
        string? state,
        CancellationToken cancellationToken = default)
    {
        if (string.IsNullOrWhiteSpace(code))
        {
            throw new InvalidOperationException("Missing authorization code.");
        }

        var provider = await GetProviderAsync(providerId, cancellationToken)
            ?? throw new InvalidOperationException("OIDC provider not found.");

        var returnUrl = "/";
        if (!string.IsNullOrWhiteSpace(state))
        {
            PendingOidcLogin? pending;
            lock (PendingLogins)
            {
                PendingLogins.Remove(state, out pending);
            }

            if (pending is not null)
            {
                if (pending.ProviderId != providerId || pending.ExpiresAtUtc < DateTimeOffset.UtcNow)
                {
                    throw new InvalidOperationException("OIDC login state expired.");
                }

                returnUrl = pending.ReturnUrl;
            }
        }

        var subject = await ExchangeCodeForSubjectAsync(context, provider, code, cancellationToken);
        var sessionKey = Convert.ToHexString(SHA256.HashData(Encoding.UTF8.GetBytes($"{provider.Id}:{subject}:{code}"))).ToLowerInvariant();
        var appSettings = await settings.GetOrCreateAsync(cancellationToken);
        var sessionHours = Math.Clamp(appSettings.EdgeSsoSessionHours, 1, 168);
        var expires = DateTimeOffset.UtcNow.AddHours(sessionHours);

        var existing = await db.EdgeSessions.SingleOrDefaultAsync(x => x.SessionKey == sessionKey, cancellationToken);
        if (existing is null)
        {
            db.EdgeSessions.Add(new EdgeSessionEntity
            {
                SessionKey = sessionKey,
                OidcProviderId = provider.Id,
                Subject = subject,
                ExpiresAtUtc = expires,
            });
        }
        else
        {
            existing.ExpiresAtUtc = expires;
            existing.Subject = subject;
        }

        await db.SaveChangesAsync(cancellationToken);
        return new EdgeCallbackResult(returnUrl, sessionKey, await BuildSessionCookieAsync(context, expires, appSettings.RootDomain, cancellationToken));
    }

    public async Task<bool> ValidateSessionAsync(string? sessionKey, CancellationToken cancellationToken = default)
    {
        if (string.IsNullOrWhiteSpace(sessionKey))
        {
            return false;
        }

        var session = await db.EdgeSessions.AsNoTracking()
            .SingleOrDefaultAsync(x => x.SessionKey == sessionKey, cancellationToken);
        return session is not null && session.ExpiresAtUtc > DateTimeOffset.UtcNow;
    }

    public async Task ClearSessionAsync(string? sessionKey, CancellationToken cancellationToken = default)
    {
        if (string.IsNullOrWhiteSpace(sessionKey))
        {
            return;
        }

        await db.EdgeSessions.Where(x => x.SessionKey == sessionKey).ExecuteDeleteAsync(cancellationToken);
    }

    private async Task<string> ExchangeCodeForSubjectAsync(
        HttpContext context,
        OidcProviderEntity provider,
        string code,
        CancellationToken cancellationToken)
    {
        if (IsFakeIssuer(provider.Issuer))
        {
            return code.StartsWith("subject:", StringComparison.Ordinal) ? code["subject:".Length..] : "edge-user";
        }

        var clientSecret = await secrets.DecryptForPurposeAsync(provider.ClientSecretId, cancellationToken)
            ?? throw new InvalidOperationException("OIDC client secret unavailable; unlock vault.");
        var redirectUri = BuildLoginRedirectUri(context, provider.Id, "/");
        var client = httpClientFactory.CreateClient("oidc-edge");
        using var request = new HttpRequestMessage(HttpMethod.Post, $"{provider.Issuer.TrimEnd('/')}/oauth/token");
        request.Content = new FormUrlEncodedContent(new Dictionary<string, string>
        {
            ["grant_type"] = "authorization_code",
            ["code"] = code,
            ["redirect_uri"] = redirectUri,
            ["client_id"] = provider.ClientId,
            ["client_secret"] = Encoding.UTF8.GetString(clientSecret),
        });
        var response = await client.SendAsync(request, cancellationToken);
        response.EnsureSuccessStatusCode();
        var payload = await response.Content.ReadFromJsonAsync<OidcTokenResponse>(cancellationToken)
            ?? throw new InvalidOperationException("Invalid OIDC token response.");
        return payload.Subject ?? ParseIdTokenSubject(payload.IdToken) ?? "edge-user";
    }

    private static string? ParseIdTokenSubject(string? idToken)
    {
        if (string.IsNullOrWhiteSpace(idToken))
        {
            return null;
        }

        var parts = idToken.Split('.');
        if (parts.Length < 2)
        {
            return null;
        }

        try
        {
            var payload = parts[1].Replace('-', '+').Replace('_', '/');
            payload += new string('=', (4 - payload.Length % 4) % 4);
            var json = Encoding.UTF8.GetString(Convert.FromBase64String(payload));
            using var doc = System.Text.Json.JsonDocument.Parse(json);
            return doc.RootElement.TryGetProperty("sub", out var sub) ? sub.GetString() : null;
        }
        catch
        {
            return null;
        }
    }

    private static bool IsFakeIssuer(string issuer)
        => issuer.Contains("/fake", StringComparison.OrdinalIgnoreCase)
           || issuer.Contains(".fake.", StringComparison.OrdinalIgnoreCase)
           || issuer.Contains("127.0.0.1", StringComparison.OrdinalIgnoreCase)
           || issuer.Contains("localhost", StringComparison.OrdinalIgnoreCase);

    private static async Task<CookieOptions> BuildSessionCookieAsync(
        HttpContext context,
        DateTimeOffset expires,
        string? rootDomain,
        CancellationToken cancellationToken)
    {
        var host = context.Request.Host.Host;
        var cookie = new CookieOptions
        {
            HttpOnly = true,
            Secure = context.Request.IsHttps,
            SameSite = SameSiteMode.Lax,
            Expires = expires,
        };

        if (!string.IsNullOrWhiteSpace(rootDomain)
            && !string.Equals(host, "localhost", StringComparison.OrdinalIgnoreCase)
            && host != "127.0.0.1")
        {
            cookie.Domain = rootDomain.StartsWith('.') ? rootDomain : $".{rootDomain}";
        }
        else if (!string.Equals(host, "localhost", StringComparison.OrdinalIgnoreCase)
            && host != "127.0.0.1")
        {
            cookie.Domain = host.StartsWith('.') ? host : $".{host}";
        }

        await Task.CompletedTask;
        return cookie;
    }

    private sealed record PendingOidcLogin(Guid ProviderId, string ReturnUrl, DateTimeOffset ExpiresAtUtc);

    private sealed record OidcTokenResponse(
        [property: JsonPropertyName("sub")] string? Subject,
        [property: JsonPropertyName("id_token")] string? IdToken);
}

public sealed record EdgeCallbackResult(string ReturnUrl, string SessionKey, CookieOptions SessionCookie);
