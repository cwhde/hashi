using System.Net;
using Hashi.Core.Auth;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Microsoft.AspNetCore.Mvc.Testing;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Xunit;

namespace Hashi.IntegrationTests;

public sealed class EdgeAuthOidcTests : IAsyncLifetime
{
    private readonly IntegrationTestPostgres _postgres = new();
    private WebApplicationFactory<Program>? _factory;
    private HttpClient? _client;

    public async Task InitializeAsync()
    {
        await _postgres.StartAsync();
        if (!_postgres.IsAvailable)
        {
            return;
        }

        Environment.SetEnvironmentVariable("HASHI_SKIP_STARTUP_HOOKS", "1");
        _factory = IntegrationTestApp.CreateFactory(_postgres.ConnectionString);
        _client = _factory.CreateClient(new WebApplicationFactoryClientOptions { HandleCookies = true });
        await IntegrationTestApp.MigrateAsync(_factory.Services);

        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<HashiDbContext>();
        var vault = scope.ServiceProvider.GetRequiredService<VaultSessionState>();
        vault.Unlock(new byte[32]);
        var secrets = scope.ServiceProvider.GetRequiredService<SecretRecordService>();
        var stored = await secrets.StoreAsync(
            SecretPurpose.OidcClientSecret,
            "Edge SSO",
            "fake-secret"u8.ToArray());
        db.OidcProviders.Add(new OidcProviderEntity
        {
            Name = "Fake IdP",
            Issuer = "https://idp.fake.local",
            ClientId = "hashi-edge",
            ClientSecretId = stored.Id,
            Enabled = true,
        });
        await db.SaveChangesAsync();
    }

    public async Task DisposeAsync()
    {
        _client?.Dispose();
        if (_factory is not null)
        {
            await _factory.DisposeAsync();
        }

        await _postgres.DisposeAsync();
    }

    [Fact]
    public async Task Callback_stores_session_and_forward_auth_allows()
    {
        if (!_postgres.IsAvailable || _factory is null || _client is null)
        {
            return;
        }

        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<HashiDbContext>();
        var provider = await db.OidcProviders.SingleAsync();

        var callback = await _client.GetAsync(
            $"/api/edge-auth/callback?providerId={provider.Id}&code=subject:test-user");
        Assert.Equal(HttpStatusCode.Redirect, callback.StatusCode);

        var forward = await _client.GetAsync("/api/edge-auth/forward");
        Assert.Equal(HttpStatusCode.NoContent, forward.StatusCode);
    }
}
