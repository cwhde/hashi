using System.Security.Claims;
using System.Security.Cryptography;
using System.Text;
using System.Text.Json;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Services;
using IdentityModel;
using IdentityModel.Client;
using IdentityModel.OidcClient;
using IdentityModel.OidcClient.Results;
using Microsoft.AspNetCore.DataProtection;
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
    IConfiguration configuration,
    IDataProtectionProvider dataProtectionProvider)
{
    private const string CorrelationCookiePrefix = "hashi.edge.oidc.";
    private static readonly TimeSpan CorrelationLifetime = TimeSpan.FromMinutes(10);
    private readonly IDataProtector _correlationProtector = dataProtectionProvider.CreateProtector("Hashi.EdgeSso.OidcCorrelation.v1");

    public async Task<IReadOnlyList<OidcProviderEntity>> ListProvidersAsync(CancellationToken cancellationToken = default)
        => await db.OidcProviders.AsNoTracking().Where(x => x.Enabled).ToListAsync(cancellationToken);

    public async Task<OidcProviderEntity?> GetProviderAsync(Guid providerId, CancellationToken cancellationToken = default)
        => await db.OidcProviders.AsNoTracking().SingleOrDefaultAsync(x => x.Id == providerId && x.Enabled, cancellationToken);

    public string BuildLoginRedirectUri(HttpContext context, Guid providerId)
    {
        var baseUrl = $"{context.Request.Scheme}://{context.Request.Host}";
        return $"{baseUrl}/api/edge-auth/callback?providerId={providerId}";
    }

    public async Task<string> BuildAuthorizationUrlAsync(
        HttpContext context,
        Guid providerId,
        string returnUrl,
        bool rememberMe = false,
        CancellationToken cancellationToken = default)
    {
        var provider = await GetProviderAsync(providerId, cancellationToken)
            ?? throw new InvalidOperationException("OIDC provider not found.");
        var nonce = CryptoRandom.CreateUniqueId(32);
        var client = await CreateClientAsync(context, provider, nonce, cancellationToken);
        var state = await client.PrepareLoginAsync(new Parameters { { "nonce", nonce } }, cancellationToken);
        if (state.IsError)
        {
            throw new InvalidOperationException($"OIDC authorization request failed: {state.Error}");
        }

        var correlation = new OidcCorrelationState(
            provider.Id,
            NormalizeReturnUrl(returnUrl),
            rememberMe,
            nonce,
            state.State,
            state.CodeVerifier,
            state.RedirectUri,
            DateTimeOffset.UtcNow.Add(CorrelationLifetime));
        context.Response.Cookies.Append(
            BuildCorrelationCookieName(state.State),
            _correlationProtector.Protect(JsonSerializer.Serialize(correlation)),
            BuildCorrelationCookieOptions(context, correlation.ExpiresAtUtc));

        return state.StartUrl;
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
        var correlation = ReadCorrelationState(context, providerId, state);
        var client = await CreateClientAsync(context, provider, correlation.Nonce, cancellationToken);
        var callbackUrl = BuildCallbackUrl(context);
        var result = await client.ProcessResponseAsync(
            callbackUrl,
            new AuthorizeState
            {
                State = correlation.State,
                CodeVerifier = correlation.CodeVerifier,
                RedirectUri = correlation.RedirectUri,
            },
            cancellationToken: cancellationToken);
        context.Response.Cookies.Delete(BuildCorrelationCookieName(correlation.State));

        if (result.IsError)
        {
            throw new InvalidOperationException($"OIDC callback validation failed: {result.Error}");
        }

        var subject = result.User.FindFirst(JwtClaimTypes.Subject)?.Value
            ?? result.User.FindFirst(ClaimTypes.NameIdentifier)?.Value;
        if (string.IsNullOrWhiteSpace(subject))
        {
            throw new InvalidOperationException("OIDC ID token is missing subject.");
        }

        var appSettings = await settings.GetOrCreateAsync(cancellationToken);
        var policy = EdgeSsoSessionPolicy.From(appSettings, correlation.RememberMe);
        var now = DateTimeOffset.UtcNow;
        var expires = now.Add(policy.AbsoluteLifetime);
        var sessionKey = Convert.ToHexString(RandomNumberGenerator.GetBytes(32)).ToLowerInvariant();
        db.EdgeSessions.Add(new EdgeSessionEntity
        {
            SessionKey = sessionKey,
            OidcProviderId = provider.Id,
            Subject = subject,
            ExpiresAtUtc = expires,
            LastSeenAtUtc = now,
            RememberMe = correlation.RememberMe && policy.RememberMeEnabled,
        });

        await db.SaveChangesAsync(cancellationToken);
        return new EdgeCallbackResult(
            correlation.ReturnUrl,
            sessionKey,
            BuildSessionCookie(context, expires, appSettings.RootDomain, persistent: correlation.RememberMe && policy.RememberMeEnabled));
    }

    public async Task<bool> ValidateSessionAsync(string? sessionKey, CancellationToken cancellationToken = default)
    {
        if (string.IsNullOrWhiteSpace(sessionKey))
        {
            return false;
        }

        var session = await db.EdgeSessions.SingleOrDefaultAsync(x => x.SessionKey == sessionKey, cancellationToken);
        if (session is null)
        {
            return false;
        }

        var appSettings = await settings.GetOrCreateAsync(cancellationToken);
        var idleTimeout = EdgeSsoSessionPolicy.IdleTimeout(appSettings);
        var now = DateTimeOffset.UtcNow;
        if (session.ExpiresAtUtc <= now || session.LastSeenAtUtc.Add(idleTimeout) <= now)
        {
            db.EdgeSessions.Remove(session);
            await db.SaveChangesAsync(cancellationToken);
            return false;
        }

        session.LastSeenAtUtc = now;
        await db.SaveChangesAsync(cancellationToken);
        return true;
    }

    public async Task ClearSessionAsync(string? sessionKey, CancellationToken cancellationToken = default)
    {
        if (string.IsNullOrWhiteSpace(sessionKey))
        {
            return;
        }

        await db.EdgeSessions.Where(x => x.SessionKey == sessionKey).ExecuteDeleteAsync(cancellationToken);
    }

    private async Task<OidcClient> CreateClientAsync(
        HttpContext context,
        OidcProviderEntity provider,
        string expectedNonce,
        CancellationToken cancellationToken)
    {
        EnsureSecureOidcUrl(provider.Issuer, "OIDC issuer");
        var clientSecret = await secrets.DecryptForPurposeAsync(provider.ClientSecretId, cancellationToken)
            ?? throw new InvalidOperationException("OIDC client secret unavailable; unlock vault.");
        var allowHttpLoopback = IsHttpLoopbackDiscoveryAllowed(provider.Issuer);
        var options = new OidcClientOptions
        {
            Authority = provider.Issuer.TrimEnd('/'),
            ClientId = provider.ClientId,
            ClientSecret = Encoding.UTF8.GetString(clientSecret),
            RedirectUri = BuildLoginRedirectUri(context, provider.Id),
            Scope = provider.Scopes,
            LoadProfile = false,
            DisablePushedAuthorization = true,
            ClockSkew = TimeSpan.FromMinutes(2),
            Policy = new Policy
            {
                RequireIdentityTokenSignature = true,
                ValidateTokenIssuerName = true,
                Discovery = new DiscoveryPolicy
                {
                    RequireHttps = !allowHttpLoopback,
                    ValidateIssuerName = true,
                    ValidateEndpoints = true,
                },
            },
            IdentityTokenValidator = new HashiIdentityTokenValidator(expectedNonce),
            HttpClientFactory = _ => httpClientFactory.CreateClient("oidc-edge"),
        };

        return new OidcClient(options);
    }

    private OidcCorrelationState ReadCorrelationState(HttpContext context, Guid providerId, string? state)
    {
        if (string.IsNullOrWhiteSpace(state))
        {
            throw new InvalidOperationException("OIDC login state is required.");
        }

        var cookieName = BuildCorrelationCookieName(state);
        if (!context.Request.Cookies.TryGetValue(cookieName, out var protectedState)
            || string.IsNullOrWhiteSpace(protectedState))
        {
            throw new InvalidOperationException("OIDC login state is unknown or expired.");
        }

        OidcCorrelationState correlation;
        try
        {
            correlation = JsonSerializer.Deserialize<OidcCorrelationState>(_correlationProtector.Unprotect(protectedState))
                ?? throw new InvalidOperationException("OIDC login state is invalid.");
        }
        catch (Exception ex) when (ex is not InvalidOperationException)
        {
            throw new InvalidOperationException("OIDC login state is invalid.", ex);
        }

        if (correlation.ProviderId != providerId
            || !string.Equals(correlation.State, state, StringComparison.Ordinal)
            || correlation.ExpiresAtUtc <= DateTimeOffset.UtcNow)
        {
            throw new InvalidOperationException("OIDC login state is unknown or expired.");
        }

        return correlation;
    }

    private static string BuildCallbackUrl(HttpContext context)
        => $"{context.Request.Scheme}://{context.Request.Host}{context.Request.PathBase}{context.Request.Path}{context.Request.QueryString}";

    private static string NormalizeReturnUrl(string returnUrl)
        => string.IsNullOrWhiteSpace(returnUrl) ? "/" : returnUrl;

    private static string BuildCorrelationCookieName(string state)
    {
        var safeState = Convert.ToHexString(SHA256.HashData(Encoding.UTF8.GetBytes(state))).ToLowerInvariant()[..32];
        return $"{CorrelationCookiePrefix}{safeState}";
    }

    private static CookieOptions BuildCorrelationCookieOptions(HttpContext context, DateTimeOffset expires)
        => new()
        {
            HttpOnly = true,
            Secure = true,
            SameSite = SameSiteMode.Lax,
            Expires = expires,
            IsEssential = true,
            Path = context.Request.PathBase.HasValue ? context.Request.PathBase.Value! : "/",
        };

    private static CookieOptions BuildSessionCookie(
        HttpContext context,
        DateTimeOffset expires,
        string? rootDomain,
        bool persistent)
    {
        var host = context.Request.Host.Host;
        var cookie = new CookieOptions
        {
            HttpOnly = true,
            Secure = true,
            SameSite = SameSiteMode.Lax,
            IsEssential = true,
            Path = "/",
        };

        if (persistent)
        {
            cookie.Expires = expires;
        }

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

        return cookie;
    }

    private bool IsHttpLoopbackDiscoveryAllowed(string issuer)
    {
        if (!configuration.GetValue<bool>("Hashi:Oidc:AllowHttpDiscovery"))
        {
            return false;
        }

        return Uri.TryCreate(issuer, UriKind.Absolute, out var uri)
            && uri.Scheme == Uri.UriSchemeHttp
            && (uri.IsLoopback || string.Equals(uri.Host, "localhost", StringComparison.OrdinalIgnoreCase));
    }

    private void EnsureSecureOidcUrl(string url, string label)
    {
        if (!Uri.TryCreate(url, UriKind.Absolute, out var uri))
        {
            throw new InvalidOperationException($"{label} must be an absolute URL.");
        }

        if (uri.Scheme == Uri.UriSchemeHttps || IsHttpLoopbackDiscoveryAllowed(url))
        {
            return;
        }

        throw new InvalidOperationException($"{label} must use HTTPS.");
    }

    private sealed record OidcCorrelationState(
        Guid ProviderId,
        string ReturnUrl,
        bool RememberMe,
        string Nonce,
        string State,
        string CodeVerifier,
        string RedirectUri,
        DateTimeOffset ExpiresAtUtc);

    private sealed class HashiIdentityTokenValidator(string expectedNonce) : IIdentityTokenValidator
    {
        public async Task<IdentityTokenValidationResult> ValidateAsync(
            string identityToken,
            OidcClientOptions options,
            CancellationToken cancellationToken = default)
        {
            await Task.CompletedTask;
            var keySetJson = options.ProviderInformation.KeySet.RawData
                ?? JsonSerializer.Serialize(options.ProviderInformation.KeySet);
            var result = await new JsonWebTokenHandler().ValidateTokenAsync(identityToken, new TokenValidationParameters
            {
                ValidateIssuer = true,
                ValidIssuer = options.ProviderInformation.IssuerName,
                ValidateAudience = true,
                ValidAudience = options.ClientId,
                ValidateIssuerSigningKey = true,
                IssuerSigningKeys = new Microsoft.IdentityModel.Tokens.JsonWebKeySet(keySetJson).GetSigningKeys(),
                ValidateLifetime = true,
                RequireExpirationTime = true,
                RequireSignedTokens = true,
                ClockSkew = options.ClockSkew,
            });

            if (!result.IsValid)
            {
                return new IdentityTokenValidationResult { Error = result.Exception?.Message ?? "invalid_token" };
            }

            var identity = result.ClaimsIdentity;
            if (identity is null)
            {
                return new IdentityTokenValidationResult { Error = "missing_identity" };
            }

            var nonce = identity.FindFirst(JwtClaimTypes.Nonce)?.Value;
            if (!string.Equals(nonce, expectedNonce, StringComparison.Ordinal))
            {
                return new IdentityTokenValidationResult { Error = "invalid_nonce" };
            }

            var token = new JsonWebTokenHandler().ReadJsonWebToken(identityToken);
            return new IdentityTokenValidationResult
            {
                User = new ClaimsPrincipal(identity),
                SignatureAlgorithm = token.Alg,
            };
        }
    }

    private sealed record EdgeSsoSessionPolicy(TimeSpan AbsoluteLifetime, bool RememberMeEnabled)
    {
        public static EdgeSsoSessionPolicy From(AppSettingsEntity settings, bool rememberMe)
        {
            var sessionHours = Math.Clamp(settings.EdgeSsoSessionHours, 1, 168);
            var rememberDays = Math.Clamp(settings.EdgeSsoRememberDeviceDays, 0, 365);
            return rememberMe && rememberDays > 0
                ? new EdgeSsoSessionPolicy(TimeSpan.FromDays(rememberDays), RememberMeEnabled: true)
                : new EdgeSsoSessionPolicy(TimeSpan.FromHours(sessionHours), RememberMeEnabled: false);
        }

        public static TimeSpan IdleTimeout(AppSettingsEntity settings)
            => TimeSpan.FromMinutes(Math.Clamp(settings.EdgeSsoIdleTimeoutMinutes, 5, 10080));
    }
}

public sealed record EdgeCallbackResult(string ReturnUrl, string SessionKey, CookieOptions SessionCookie);
