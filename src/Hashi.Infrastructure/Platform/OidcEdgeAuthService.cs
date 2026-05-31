using System.Net.Http.Json;
using System.Security.Cryptography;
using System.Text;
using System.Text.Json;
using System.Text.Json.Serialization;
using Hashi.Contracts.Api;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Services;
using Microsoft.AspNetCore.Http;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Configuration;
using Microsoft.IdentityModel.JsonWebTokens;
using Microsoft.IdentityModel.Tokens;

namespace Hashi.Infrastructure.Platform;

public sealed class OidcEdgeAuthService(
    HashiDbContext db,
    SecretRecordService secrets,
    IHttpClientFactory httpClientFactory,
    AppSettingsService settings,
    IConfiguration configuration)
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
        var nonce = Convert.ToHexString(RandomNumberGenerator.GetBytes(16)).ToLowerInvariant();
        var redirectUri = BuildLoginRedirectUri(context, provider.Id, returnUrl);
        lock (PendingLogins)
        {
            PendingLogins[state] = new PendingOidcLogin(provider.Id, returnUrl, nonce, DateTimeOffset.UtcNow.AddMinutes(10));
        }

        var authorizationEndpoint = IsUnsignedTestIssuerAllowed(provider.Issuer)
            ? $"{provider.Issuer.TrimEnd('/')}/oauth/authorize"
            : (await GetDiscoveryAsync(provider, cancellationToken)).AuthorizationEndpoint;
        var scope = Uri.EscapeDataString(provider.Scopes);
        var callback = Uri.EscapeDataString(redirectUri);
        return $"{authorizationEndpoint}?client_id={Uri.EscapeDataString(provider.ClientId)}&redirect_uri={callback}&response_type=code&scope={scope}&state={state}&nonce={Uri.EscapeDataString(nonce)}";
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

        if (string.IsNullOrWhiteSpace(state))
        {
            throw new InvalidOperationException("OIDC login state is required.");
        }

        PendingOidcLogin? pending;
        lock (PendingLogins)
        {
            PendingLogins.Remove(state, out pending);
        }

        if (pending is null)
        {
            throw new InvalidOperationException("OIDC login state is unknown or expired.");
        }

        if (pending.ProviderId != providerId || pending.ExpiresAtUtc < DateTimeOffset.UtcNow)
        {
            throw new InvalidOperationException("OIDC login state expired.");
        }

        var returnUrl = pending.ReturnUrl;
        var subject = await ExchangeCodeForSubjectAsync(context, provider, code, pending.Nonce, cancellationToken);
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
        string nonce,
        CancellationToken cancellationToken)
    {
        if (IsUnsignedTestIssuerAllowed(provider.Issuer))
        {
            return code.StartsWith("subject:", StringComparison.Ordinal) ? code["subject:".Length..] : "edge-user";
        }

        var clientSecret = await secrets.DecryptForPurposeAsync(provider.ClientSecretId, cancellationToken)
            ?? throw new InvalidOperationException("OIDC client secret unavailable; unlock vault.");
        var redirectUri = BuildLoginRedirectUri(context, provider.Id, "/");
        var client = httpClientFactory.CreateClient("oidc-edge");
        var discovery = await GetDiscoveryAsync(provider, cancellationToken);
        using var request = new HttpRequestMessage(HttpMethod.Post, discovery.TokenEndpoint);
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
        return await ValidateIdTokenAsync(provider, discovery, payload.IdToken, nonce, cancellationToken);
    }

    private async Task<string> ValidateIdTokenAsync(
        OidcProviderEntity provider,
        OidcDiscoveryDocument discovery,
        string? idToken,
        string expectedNonce,
        CancellationToken cancellationToken)
    {
        if (string.IsNullOrWhiteSpace(idToken))
        {
            throw new InvalidOperationException("OIDC token response did not include an ID token.");
        }

        var client = httpClientFactory.CreateClient("oidc-edge");
        var jwks = await client.GetStringAsync(discovery.JwksUri, cancellationToken);
        var keys = new JsonWebKeySet(jwks).GetSigningKeys();
        var result = await new JsonWebTokenHandler().ValidateTokenAsync(idToken, new TokenValidationParameters
        {
            ValidateIssuer = true,
            ValidIssuer = provider.Issuer.TrimEnd('/'),
            ValidateAudience = true,
            ValidAudience = provider.ClientId,
            ValidateIssuerSigningKey = true,
            IssuerSigningKeys = keys,
            ValidateLifetime = true,
            RequireExpirationTime = true,
            RequireSignedTokens = true,
            ClockSkew = TimeSpan.FromMinutes(2),
        });

        if (!result.IsValid)
        {
            throw new InvalidOperationException("Invalid OIDC ID token.", result.Exception);
        }

        var subject = result.ClaimsIdentity?.FindFirst("sub")?.Value;
        if (string.IsNullOrWhiteSpace(subject))
        {
            throw new InvalidOperationException("OIDC ID token is missing subject.");
        }

        var actualNonce = result.ClaimsIdentity?.FindFirst("nonce")?.Value;
        if (!string.Equals(actualNonce, expectedNonce, StringComparison.Ordinal))
        {
            throw new InvalidOperationException("OIDC ID token nonce mismatch.");
        }

        return subject;
    }

    private async Task<OidcDiscoveryDocument> GetDiscoveryAsync(
        OidcProviderEntity provider,
        CancellationToken cancellationToken)
    {
        EnsureSecureOidcUrl(provider.Issuer, "OIDC issuer");
        var client = httpClientFactory.CreateClient("oidc-edge");
        var discoveryUrl = $"{provider.Issuer.TrimEnd('/')}/.well-known/openid-configuration";
        using var response = await client.GetAsync(discoveryUrl, cancellationToken);
        response.EnsureSuccessStatusCode();
        await using var stream = await response.Content.ReadAsStreamAsync(cancellationToken);
        using var doc = await JsonDocument.ParseAsync(stream, cancellationToken: cancellationToken);
        var root = doc.RootElement;
        var authorizationEndpoint = ReadRequiredString(root, "authorization_endpoint");
        var tokenEndpoint = ReadRequiredString(root, "token_endpoint");
        var jwksUri = ReadRequiredString(root, "jwks_uri");
        EnsureSecureOidcUrl(authorizationEndpoint, "OIDC authorization endpoint");
        EnsureSecureOidcUrl(tokenEndpoint, "OIDC token endpoint");
        EnsureSecureOidcUrl(jwksUri, "OIDC JWKS endpoint");
        return new OidcDiscoveryDocument(authorizationEndpoint, tokenEndpoint, jwksUri);
    }

    private static string ReadRequiredString(JsonElement root, string propertyName)
    {
        if (!root.TryGetProperty(propertyName, out var value) || value.ValueKind != JsonValueKind.String)
        {
            throw new InvalidOperationException($"OIDC discovery document is missing {propertyName}.");
        }

        return value.GetString() ?? throw new InvalidOperationException($"OIDC discovery document is missing {propertyName}.");
    }

    private void EnsureSecureOidcUrl(string url, string label)
    {
        if (!Uri.TryCreate(url, UriKind.Absolute, out var uri))
        {
            throw new InvalidOperationException($"{label} must be an absolute URL.");
        }

        if (uri.Scheme == Uri.UriSchemeHttps)
        {
            return;
        }

        var allowHttp = configuration.GetValue<bool>("Hashi:Oidc:AllowHttpDiscovery");
        if (allowHttp && (uri.IsLoopback || string.Equals(uri.Host, "localhost", StringComparison.OrdinalIgnoreCase)))
        {
            return;
        }

        throw new InvalidOperationException($"{label} must use HTTPS.");
    }

    private bool IsUnsignedTestIssuerAllowed(string issuer)
        => configuration.GetValue<bool>("Hashi:Oidc:AllowUnsignedTestTokens") && IsFakeIssuer(issuer);

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

    private sealed record PendingOidcLogin(Guid ProviderId, string ReturnUrl, string Nonce, DateTimeOffset ExpiresAtUtc);

    private sealed record OidcDiscoveryDocument(string AuthorizationEndpoint, string TokenEndpoint, string JwksUri);

    private sealed record OidcTokenResponse(
        [property: JsonPropertyName("sub")] string? Subject,
        [property: JsonPropertyName("id_token")] string? IdToken);
}

public sealed record EdgeCallbackResult(string ReturnUrl, string SessionKey, CookieOptions SessionCookie);
