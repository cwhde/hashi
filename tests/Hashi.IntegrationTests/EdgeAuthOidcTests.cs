using System.Net;
using Hashi.Core.Auth;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Platform;
using Microsoft.AspNetCore.Mvc.Testing;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Testcontainers.PostgreSql;
using Xunit;

namespace Hashi.IntegrationTests;

public sealed class EdgeAuthOidcTests : IAsyncLifetime
{
    private readonly PostgreSqlContainer _postgres = new PostgreSqlBuilder()
        .WithImage("postgres:18")
        .WithDatabase("hashi")
        .WithUsername("hashi")
        .WithPassword("hashi")
        .Build();

    private WebApplicationFactory<Program> _factory = null!;
    private HttpClient _client = null!;
    private bool _dockerUnavailable;

    public async Task InitializeAsync()
    {
        if (!File.Exists("/var/run/docker.sock"))
        {
            _dockerUnavailable = true;
            return;
        }

        Environment.SetEnvironmentVariable("HASHI_SKIP_STARTUP_HOOKS", "1");
        await _postgres.StartAsync();
        _factory = new WebApplicationFactory<Program>()
            .WithWebHostBuilder(builder =>
            {
                builder.UseSetting("ConnectionStrings:Hashi", _postgres.GetConnectionString());
                builder.UseSetting("Hashi:SkipStartupHooks", "true");
            });
        _client = _factory.CreateClient(new WebApplicationFactoryClientOptions { HandleCookies = true });

        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<HashiDbContext>();
        await db.Database.MigrateAsync();
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
        _client.Dispose();
        await _factory.DisposeAsync();
        await _postgres.DisposeAsync();
    }

    [Fact]
    public async Task Callback_stores_session_and_forward_auth_allows()
    {
        if (_dockerUnavailable)
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
