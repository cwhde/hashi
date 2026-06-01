using System.Net;
using System.Security.Cryptography;
using System.Text.Json;
using Hashi.Core.Auth;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Platform;
using Hashi.Infrastructure.Services;
using IdentityModel;
using Microsoft.AspNetCore.DataProtection;
using Microsoft.AspNetCore.Http;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Configuration;
using Microsoft.IdentityModel.JsonWebTokens;
using Microsoft.IdentityModel.Tokens;
using Xunit;

namespace Hashi.UnitTests;

public sealed class OidcEdgeAuthServiceTests
{
    [Fact]
    public async Task Authorization_url_uses_pkce_and_secure_correlation_cookie()
    {
        await using var fixture = await CreateFixtureAsync();
        var context = HttpContext("https", "app.example.com");

        var authorizationUrl = await fixture.Service.BuildAuthorizationUrlAsync(context, fixture.ProviderId, "/dashboard");

        var location = new Uri(authorizationUrl);
        Assert.Equal($"{fixture.Oidc.Issuer}/authorize", location.GetLeftPart(UriPartial.Path));
        Assert.Equal("S256", GetQueryValue(location, "code_challenge_method"));
        Assert.False(string.IsNullOrWhiteSpace(GetQueryValue(location, "code_challenge")));
        AssertSetCookieFlag(context, "hashi.edge.oidc.", "Secure");
        AssertSetCookieFlag(context, "hashi.edge.oidc.", "HttpOnly");
    }

    [Fact]
    public async Task Callback_rejects_missing_correlation_cookie()
    {
        await using var fixture = await CreateFixtureAsync();
        var context = HttpContext("https", "app.example.com");

        await Assert.ThrowsAsync<InvalidOperationException>(() =>
            fixture.Service.CompleteCallbackAsync(context, fixture.ProviderId, "valid-code", "unknown-state"));
    }

    [Fact]
    public async Task Callback_rejects_invalid_nonce()
    {
        await using var fixture = await CreateFixtureAsync();
        var loginContext = HttpContext("https", "app.example.com");
        var authorizationUrl = await fixture.Service.BuildAuthorizationUrlAsync(loginContext, fixture.ProviderId, "/");
        var state = GetQueryValue(new Uri(authorizationUrl), "state");
        fixture.Oidc.SetSubjectForCode("wrong-nonce-code", "user", "wrong-nonce");
        var callbackContext = CallbackContext(loginContext, state, "wrong-nonce-code");

        await Assert.ThrowsAsync<InvalidOperationException>(() =>
            fixture.Service.CompleteCallbackAsync(callbackContext, fixture.ProviderId, "wrong-nonce-code", state));
    }

    [Fact]
    public async Task Callback_creates_remembered_root_domain_session_with_configured_lifetime()
    {
        await using var fixture = await CreateFixtureAsync(settings =>
        {
            settings.RootDomain = "example.com";
            settings.EdgeSsoSessionHours = 2;
            settings.EdgeSsoIdleTimeoutMinutes = 15;
            settings.EdgeSsoRememberDeviceDays = 14;
        });
        var loginContext = HttpContext("https", "app.example.com");
        var authorizationUrl = await fixture.Service.BuildAuthorizationUrlAsync(loginContext, fixture.ProviderId, "/dashboard", rememberMe: true);
        var location = new Uri(authorizationUrl);
        var state = GetQueryValue(location, "state");
        fixture.Oidc.SetSubjectForCode("valid-code", "edge-user", GetQueryValue(location, "nonce"));
        var callbackContext = CallbackContext(loginContext, state, "valid-code");

        var result = await fixture.Service.CompleteCallbackAsync(callbackContext, fixture.ProviderId, "valid-code", state);

        Assert.Equal("/dashboard", result.ReturnUrl);
        Assert.True(result.SessionCookie.Expires is not null);
        Assert.Equal(".example.com", result.SessionCookie.Domain);
        var session = await fixture.Db.EdgeSessions.SingleAsync();
        Assert.True(session.RememberMe);
        Assert.True(session.ExpiresAtUtc > DateTimeOffset.UtcNow.AddDays(13));
        Assert.True(session.ExpiresAtUtc < DateTimeOffset.UtcNow.AddDays(15));
    }

    [Fact]
    public async Task Validate_session_enforces_idle_timeout()
    {
        await using var fixture = await CreateFixtureAsync(settings => settings.EdgeSsoIdleTimeoutMinutes = 30);
        fixture.Db.EdgeSessions.Add(new EdgeSessionEntity
        {
            SessionKey = "idle-session",
            OidcProviderId = fixture.ProviderId,
            Subject = "edge-user",
            ExpiresAtUtc = DateTimeOffset.UtcNow.AddHours(1),
            LastSeenAtUtc = DateTimeOffset.UtcNow.AddMinutes(-31),
        });
        await fixture.Db.SaveChangesAsync();

        var valid = await fixture.Service.ValidateSessionAsync("idle-session");

        Assert.False(valid);
        Assert.Empty(fixture.Db.EdgeSessions);
    }

    private static async Task<ServiceFixture> CreateFixtureAsync(Action<AppSettingsEntity>? configureSettings = null)
    {
        var db = new HashiDbContext(new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options);
        var settings = new AppSettingsEntity();
        configureSettings?.Invoke(settings);
        db.AppSettings.Add(settings);

        var vault = new VaultSessionState();
        vault.UnlockForSession("local-test-session", new byte[32]);
        var secrets = new SecretRecordService(db, vault, new ServiceSyncVaultState());
        var stored = await secrets.StoreAsync(SecretPurpose.OidcClientSecret, "OIDC", "secret"u8.ToArray());
        var oidc = new TestOidcServer("https://idp.fake.local", "hashi-edge");
        var provider = new OidcProviderEntity
        {
            Name = "Test IdP",
            Issuer = oidc.Issuer,
            ClientId = oidc.ClientId,
            ClientSecretId = stored.Id,
            Enabled = true,
        };
        db.OidcProviders.Add(provider);
        await db.SaveChangesAsync();

        var service = new OidcEdgeAuthService(
            db,
            secrets,
            new StaticHttpClientFactory(new HttpClient(oidc.CreateHandler())),
            new AppSettingsService(db),
            new ConfigurationBuilder().Build(),
            new EphemeralDataProtectionProvider());
        return new ServiceFixture(db, service, oidc, provider.Id);
    }

    private static DefaultHttpContext HttpContext(string scheme, string host)
        => new()
        {
            Request =
            {
                Scheme = scheme,
                Host = new HostString(host),
                Path = "/api/edge-auth/login",
            },
        };

    private static DefaultHttpContext CallbackContext(DefaultHttpContext loginContext, string state, string code)
    {
        var callback = HttpContext("https", loginContext.Request.Host.Value!);
        callback.Request.Path = "/api/edge-auth/callback";
        callback.Request.QueryString = new QueryString($"?providerId=test&code={Uri.EscapeDataString(code)}&state={Uri.EscapeDataString(state)}");
        callback.Request.Headers.Cookie = loginContext.Response.Headers.SetCookie.ToArray()
            .First(value => value is not null && value.StartsWith("hashi.edge.oidc.", StringComparison.OrdinalIgnoreCase))!
            .Split(';', 2)[0];
        return callback;
    }

    private static string GetQueryValue(Uri uri, string key)
    {
        var query = uri.Query.TrimStart('?').Split('&', StringSplitOptions.RemoveEmptyEntries);
        foreach (var pair in query)
        {
            var parts = pair.Split('=', 2);
            if (parts.Length == 2 && string.Equals(Uri.UnescapeDataString(parts[0]), key, StringComparison.Ordinal))
            {
                return Uri.UnescapeDataString(parts[1]);
            }
        }

        throw new InvalidOperationException($"Missing query value '{key}'.");
    }

    private static void AssertSetCookieFlag(DefaultHttpContext context, string cookiePrefix, string flag)
    {
        var cookie = context.Response.Headers.SetCookie.ToArray().First(value =>
            value is not null && value.StartsWith(cookiePrefix, StringComparison.OrdinalIgnoreCase))!;
        Assert.Contains(
            cookie.Split(';', StringSplitOptions.TrimEntries | StringSplitOptions.RemoveEmptyEntries),
            value => string.Equals(value, flag, StringComparison.OrdinalIgnoreCase));
    }

    private sealed record ServiceFixture(
        HashiDbContext Db,
        OidcEdgeAuthService Service,
        TestOidcServer Oidc,
        Guid ProviderId) : IAsyncDisposable
    {
        public async ValueTask DisposeAsync()
        {
            await Db.DisposeAsync();
            Oidc.Dispose();
        }
    }

    private sealed class StaticHttpClientFactory(HttpClient client) : IHttpClientFactory
    {
        public HttpClient CreateClient(string name) => client;
    }

    private sealed class TestOidcServer(string issuer, string clientId) : IDisposable
    {
        private readonly RSA _rsa = RSA.Create(2048);
        private readonly Dictionary<string, (string Subject, string Nonce)> _codes = new(StringComparer.Ordinal);

        public string Issuer { get; } = issuer;

        public string ClientId { get; } = clientId;

        public void SetSubjectForCode(string code, string subject, string nonce)
        {
            _codes[code] = (subject, nonce);
        }

        public HttpMessageHandler CreateHandler() => new Handler(this);

        public void Dispose()
        {
            _rsa.Dispose();
        }

        private string CreateIdToken(string subject, string nonce)
        {
            var key = new RsaSecurityKey(_rsa) { KeyId = "hashi-test-key" };
            return new JsonWebTokenHandler().CreateToken(new SecurityTokenDescriptor
            {
                Issuer = Issuer,
                Audience = ClientId,
                IssuedAt = DateTime.UtcNow.AddMinutes(-1),
                NotBefore = DateTime.UtcNow.AddMinutes(-1),
                Expires = DateTime.UtcNow.AddMinutes(5),
                Claims = new Dictionary<string, object>
                {
                    [JwtClaimTypes.Subject] = subject,
                    [JwtClaimTypes.Nonce] = nonce,
                },
                SigningCredentials = new SigningCredentials(key, SecurityAlgorithms.RsaSha256),
            });
        }

        private string JwksJson()
        {
            var parameters = _rsa.ExportParameters(false);
            return JsonSerializer.Serialize(new
            {
                keys = new[]
                {
                    new
                    {
                        kty = "RSA",
                        kid = "hashi-test-key",
                        use = "sig",
                        alg = "RS256",
                        n = Base64UrlEncoder.Encode(parameters.Modulus),
                        e = Base64UrlEncoder.Encode(parameters.Exponent),
                    },
                },
            });
        }

        private sealed class Handler(TestOidcServer server) : HttpMessageHandler
        {
            protected override async Task<HttpResponseMessage> SendAsync(HttpRequestMessage request, CancellationToken cancellationToken)
            {
                if (request.RequestUri!.AbsolutePath.EndsWith("/.well-known/openid-configuration", StringComparison.Ordinal))
                {
                    return Json(new
                    {
                        issuer = server.Issuer,
                        authorization_endpoint = $"{server.Issuer}/authorize",
                        token_endpoint = $"{server.Issuer}/token",
                        jwks_uri = $"{server.Issuer}/jwks",
                    });
                }

                if (request.RequestUri.AbsolutePath.EndsWith("/jwks", StringComparison.Ordinal))
                {
                    return Text(server.JwksJson());
                }

                if (request.RequestUri.AbsolutePath.EndsWith("/token", StringComparison.Ordinal))
                {
                    var content = await request.Content!.ReadAsStringAsync(cancellationToken);
                    var form = content.Split('&', StringSplitOptions.RemoveEmptyEntries)
                        .Select(pair => pair.Split('=', 2))
                        .ToDictionary(
                            parts => Uri.UnescapeDataString(parts[0].Replace("+", " ")),
                            parts => parts.Length == 2 ? Uri.UnescapeDataString(parts[1].Replace("+", " ")) : string.Empty,
                            StringComparer.Ordinal);
                    var (subject, nonce) = server._codes[form["code"]];
                    return Json(new
                    {
                        id_token = server.CreateIdToken(subject, nonce),
                        access_token = "access-token",
                        token_type = "Bearer",
                        expires_in = 3600,
                    });
                }

                return new HttpResponseMessage(HttpStatusCode.NotFound);
            }

            private static HttpResponseMessage Json(object payload) => Text(JsonSerializer.Serialize(payload));

            private static HttpResponseMessage Text(string body)
                => new(HttpStatusCode.OK)
                {
                    Content = new StringContent(body, System.Text.Encoding.UTF8, "application/json"),
                };
        }
    }
}
