using System.Net;
using System.Net.Http.Json;
using Hashi.Contracts.Api;
using Microsoft.AspNetCore.Mvc.Testing;
using Testcontainers.PostgreSql;
using Xunit;

namespace Hashi.IntegrationTests;

public sealed class VaultStatusTests : IAsyncLifetime
{
    private readonly PostgreSqlContainer _postgres = new PostgreSqlBuilder()
        .WithImage("postgres:18")
        .WithDatabase("hashi")
        .WithUsername("hashi")
        .WithPassword("hashi")
        .Build();

    private WebApplicationFactory<Program> _factory = null!;
    private HttpClient _client = null!;

    public async Task InitializeAsync()
    {
        await _postgres.StartAsync();
        _factory = new WebApplicationFactory<Program>()
            .WithWebHostBuilder(builder =>
            {
                builder.UseSetting("ConnectionStrings:Hashi", _postgres.GetConnectionString());
            });
        _client = _factory.CreateClient();
    }

    public async Task DisposeAsync()
    {
        _client.Dispose();
        await _factory.DisposeAsync();
        await _postgres.DisposeAsync();
    }

    [Fact]
    public async Task Vault_status_starts_not_configured()
    {
        var status = await _client.GetFromJsonAsync<VaultStatusResponse>("/api/vault/status");
        Assert.NotNull(status);
        Assert.Equal("NotConfigured", status.LockState);
        Assert.False(status.IsVaultConfigured);
        Assert.False(status.HasPasskey);
    }

    [Fact]
    public async Task Recovery_key_generation_returns_formatted_key()
    {
        var response = await _client.PostAsync("/api/vault/recovery-key/generate", null);
        Assert.Equal(HttpStatusCode.OK, response.StatusCode);

        var payload = await response.Content.ReadFromJsonAsync<VaultGenerateRecoveryKeyResponse>();
        Assert.NotNull(payload);
        Assert.Contains('-', payload.RecoveryKey);
        Assert.True(payload.RecoveryKey.Length >= 35);
    }

    [Fact]
    public async Task Passkey_and_vault_setup_steps_require_completion_endpoint()
    {
        var blocked = await _client.PostAsync("/api/setup/steps/passkey-and-vault/complete", null);
        Assert.Equal(HttpStatusCode.BadRequest, blocked.StatusCode);
    }
}
