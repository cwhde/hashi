using System.Net;
using System.Security.Cryptography;
using System.Text.Json;
using Hashi.Core.Auth;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using IdentityModel;
using Microsoft.AspNetCore.Http;
using Microsoft.AspNetCore.Mvc.Testing;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.IdentityModel.JsonWebTokens;
using Microsoft.IdentityModel.Tokens;
using Xunit;

namespace Hashi.IntegrationTests;

[Collection(PostgresIntegrationCollection.Name)]
public sealed class EdgeAuthOidcTests : IAsyncLifetime
{
    private readonly PostgresIntegrationFixture _fixture;
    private readonly TestOidcServer _oidc = new("https://idp.fake.local", "hashi-edge");
    private WebApplicationFactory<Program>? _factory;
    private HttpClient? _client;

    public EdgeAuthOidcTests(PostgresIntegrationFixture fixture)
    {
        _fixture = fixture;
    }

    public async Task InitializeAsync()
    {
        if (!_fixture.IsAvailable)
        {
            return;
        }

        Environment.SetEnvironmentVariable("HASHI_SKIP_STARTUP_HOOKS", "1");
        var connectionString = await _fixture.CreateDatabaseAsync();
        _factory = IntegrationTestApp.CreateFactory(
            connectionString,
            services => services.AddHttpClient("oidc-edge").ConfigurePrimaryHttpMessageHandler(_ => _oidc.CreateHandler()));
        _client = _factory.CreateClient(IntegrationTestApp.HttpsClientOptions(allowAutoRedirect: false));

        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<HashiDbContext>();
        var vault = scope.ServiceProvider.GetRequiredService<VaultSessionState>();
        var serviceSync = scope.ServiceProvider.GetRequiredService<ServiceSyncVaultState>();
        serviceSync.Initialize(new byte[32]);
        var sessionId = Guid.NewGuid().ToString("N");
        vault.UnlockForSession(sessionId, new byte[32]);
        var accessor = scope.ServiceProvider.GetRequiredService<IHttpContextAccessor>();
        accessor.HttpContext = IntegrationTestAuth.CreateAdminHttpContext(sessionId);
        StoredSecretDescriptor stored;
        try
        {
            stored = await scope.ServiceProvider.GetRequiredService<SecretRecordService>().StoreAsync(
                SecretPurpose.OidcClientSecret,
                "Edge SSO",
                "fake-secret"u8.ToArray(),
                serviceSyncEligible: true);
        }
        finally
        {
            accessor.HttpContext = null;
        }

        db.OidcProviders.Add(new OidcProviderEntity
        {
            Name = "Fake IdP",
            Issuer = _oidc.Issuer,
            ClientId = _oidc.ClientId,
            ClientSecretId = stored.Id,
            Enabled = true,
        });
        await db.SaveChangesAsync();
        IntegrationTestAuth.AuthenticateAsAdminSession(_client, _factory.Services, sessionId, unlockVault: true);
    }

    public async Task DisposeAsync()
    {
        _client?.Dispose();
        _oidc.Dispose();
        if (_factory is not null)
        {
            await _factory.DisposeAsync();
        }
    }

    [Fact]
    public async Task Callback_stores_session_and_forward_auth_allows()
    {
        if (!_fixture.IsAvailable || _factory is null || _client is null)
        {
            return;
        }

        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<HashiDbContext>();
        var provider = await db.OidcProviders.SingleAsync();

        var login = await _client.GetAsync($"/api/edge-auth/login?providerId={provider.Id}&returnUrl=%2F");
        Assert.Equal(HttpStatusCode.Redirect, login.StatusCode);
        Assert.Contains("code_challenge=", login.Headers.Location!.Query, StringComparison.Ordinal);
        Assert.Contains("code_challenge_method=S256", login.Headers.Location.Query, StringComparison.Ordinal);
        var state = GetQueryValue(login.Headers.Location, "state");
        var nonce = GetQueryValue(login.Headers.Location, "nonce");
        _oidc.SetSubjectForCode("valid-code", "test-user", nonce);

        var callback = await _client.GetAsync(
            $"/api/edge-auth/callback?providerId={provider.Id}&code=valid-code&state={Uri.EscapeDataString(state)}");
        Assert.Equal(HttpStatusCode.Redirect, callback.StatusCode);
        AssertSetCookieFlag(callback, "hashi.edge.session", "Secure");
        AssertSetCookieFlag(callback, "hashi.edge.session", "HttpOnly");
        Assert.NotNull(_oidc.LastTokenRequest);
        Assert.True(_oidc.LastTokenRequest!.TryGetValue("code_verifier", out var codeVerifier));
        Assert.False(string.IsNullOrWhiteSpace(codeVerifier));

        var forward = await _client.GetAsync("/api/edge-auth/forward");
        Assert.Equal(HttpStatusCode.NoContent, forward.StatusCode);
    }

    [Fact]
    public async Task Callback_rejects_missing_state()
    {
        if (!_fixture.IsAvailable || _factory is null || _client is null)
        {
            return;
        }

        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<HashiDbContext>();
        var provider = await db.OidcProviders.SingleAsync();

        var callback = await _client.GetAsync(
            $"/api/edge-auth/callback?providerId={provider.Id}&code=valid-code");

        Assert.Equal(HttpStatusCode.BadRequest, callback.StatusCode);
    }

    [Fact]
    public async Task Callback_rejects_invalid_nonce()
    {
        if (!_fixture.IsAvailable || _factory is null || _client is null)
        {
            return;
        }

        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<HashiDbContext>();
        var provider = await db.OidcProviders.SingleAsync();

        var login = await _client.GetAsync($"/api/edge-auth/login?providerId={provider.Id}&returnUrl=%2F");
        Assert.Equal(HttpStatusCode.Redirect, login.StatusCode);
        var state = GetQueryValue(login.Headers.Location!, "state");
        _oidc.SetSubjectForCode("wrong-nonce-code", "test-user", "not-the-login-nonce");

        var callback = await _client.GetAsync(
            $"/api/edge-auth/callback?providerId={provider.Id}&code=wrong-nonce-code&state={Uri.EscapeDataString(state)}");

        Assert.Equal(HttpStatusCode.BadRequest, callback.StatusCode);
    }

    [Fact]
    public async Task Remembered_session_uses_policy_and_root_domain_cookie()
    {
        if (!_fixture.IsAvailable || _factory is null)
        {
            return;
        }

        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<HashiDbContext>();
        var settings = await db.AppSettings.SingleAsync();
        settings.RootDomain = "example.com";
        settings.EdgeSsoSessionHours = 2;
        settings.EdgeSsoIdleTimeoutMinutes = 15;
        settings.EdgeSsoRememberDeviceDays = 14;
        await db.SaveChangesAsync();
        var provider = await db.OidcProviders.SingleAsync();

        using var client = _factory.CreateClient(new WebApplicationFactoryClientOptions
        {
            BaseAddress = new Uri("https://app.example.com"),
            HandleCookies = true,
            AllowAutoRedirect = false,
        });

        var login = await client.GetAsync($"/api/edge-auth/login?providerId={provider.Id}&returnUrl=%2F&rememberMe=true");
        Assert.Equal(HttpStatusCode.Redirect, login.StatusCode);
        var state = GetQueryValue(login.Headers.Location!, "state");
        var nonce = GetQueryValue(login.Headers.Location!, "nonce");
        _oidc.SetSubjectForCode("remember-code", "remembered-user", nonce);

        var callback = await client.GetAsync(
            $"/api/edge-auth/callback?providerId={provider.Id}&code=remember-code&state={Uri.EscapeDataString(state)}");

        Assert.Equal(HttpStatusCode.Redirect, callback.StatusCode);
        var session = await db.EdgeSessions.AsNoTracking().SingleAsync(x => x.Subject == "remembered-user");
        Assert.True(session.RememberMe);
        Assert.True(session.ExpiresAtUtc > DateTimeOffset.UtcNow.AddDays(13));
        Assert.True(session.ExpiresAtUtc < DateTimeOffset.UtcNow.AddDays(15));
        AssertSetCookieFlag(callback, "hashi.edge.session", "domain=.example.com");
        AssertSetCookieFlag(callback, "hashi.edge.session", "Secure");
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

    private static void AssertSetCookieFlag(HttpResponseMessage response, string cookieName, string flag)
    {
        Assert.True(response.Headers.TryGetValues("Set-Cookie", out var values));
        var cookie = values.FirstOrDefault(value =>
            value.StartsWith($"{cookieName}=", StringComparison.OrdinalIgnoreCase));
        Assert.NotNull(cookie);
        Assert.Contains(
            cookie!.Split(';', StringSplitOptions.TrimEntries | StringSplitOptions.RemoveEmptyEntries),
            value => string.Equals(value, flag, StringComparison.OrdinalIgnoreCase));
    }

    private sealed class TestOidcServer(string issuer, string clientId) : IDisposable
    {
        private readonly RSA _rsa = RSA.Create(2048);
        private readonly Dictionary<string, (string Subject, string Nonce)> _codes = new(StringComparer.Ordinal);

        public string Issuer { get; } = issuer;

        public string ClientId { get; } = clientId;

        public IReadOnlyDictionary<string, string>? LastTokenRequest { get; private set; }

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
            var token = new JsonWebTokenHandler().CreateToken(new SecurityTokenDescriptor
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
            return token;
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
                    return new HttpResponseMessage(HttpStatusCode.OK)
                    {
                        Content = new StringContent(server.JwksJson(), System.Text.Encoding.UTF8, "application/json"),
                    };
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
                    server.LastTokenRequest = form;
                    var code = form["code"];
                    var (subject, nonce) = server._codes[code];
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

            private static HttpResponseMessage Json(object payload)
                => new(HttpStatusCode.OK)
                {
                    Content = new StringContent(JsonSerializer.Serialize(payload), System.Text.Encoding.UTF8, "application/json"),
                };
        }
    }
}
