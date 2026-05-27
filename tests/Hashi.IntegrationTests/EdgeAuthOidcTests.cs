using System.Net;
using Hashi.Core.Auth;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Microsoft.AspNetCore.Mvc.Testing;
using Microsoft.AspNetCore.Http;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Xunit;

namespace Hashi.IntegrationTests;

[Collection(PostgresIntegrationCollection.Name)]
public sealed class EdgeAuthOidcTests : IAsyncLifetime
{
    private readonly PostgresIntegrationFixture _fixture;
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
        _factory = IntegrationTestApp.CreateFactory(connectionString);
        _client = _factory.CreateClient(new WebApplicationFactoryClientOptions
        {
            HandleCookies = true,
            AllowAutoRedirect = false,
        });

        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<HashiDbContext>();
        var vault = scope.ServiceProvider.GetRequiredService<VaultSessionState>();
        var sessionId = Guid.NewGuid().ToString("N");
        vault.UnlockForSession(sessionId, new byte[32]);
        var accessor = scope.ServiceProvider.GetRequiredService<IHttpContextAccessor>();
        accessor.HttpContext = IntegrationTestAuth.CreateAdminHttpContext(sessionId);
        var secrets = scope.ServiceProvider.GetRequiredService<SecretRecordService>();
        StoredSecretDescriptor stored;
        try
        {
            stored = await secrets.StoreAsync(
                SecretPurpose.OidcClientSecret,
                "Edge SSO",
                "fake-secret"u8.ToArray());
        }
        finally
        {
            accessor.HttpContext = null;
        }
        db.OidcProviders.Add(new OidcProviderEntity
        {
            Name = "Fake IdP",
            Issuer = "https://idp.fake.local",
            ClientId = "hashi-edge",
            ClientSecretId = stored.Id,
            Enabled = true,
        });
        await db.SaveChangesAsync();
        IntegrationTestAuth.AuthenticateAsAdminSession(_client, _factory.Services, sessionId, unlockVault: true);
    }

    public async Task DisposeAsync()
    {
        _client?.Dispose();
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
        var state = GetQueryValue(login.Headers.Location!, "state");

        var callback = await _client.GetAsync(
            $"/api/edge-auth/callback?providerId={provider.Id}&code=subject:test-user&state={Uri.EscapeDataString(state)}");
        Assert.Equal(HttpStatusCode.Redirect, callback.StatusCode);

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
            $"/api/edge-auth/callback?providerId={provider.Id}&code=subject:test-user");

        Assert.Equal(HttpStatusCode.BadRequest, callback.StatusCode);
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
}
