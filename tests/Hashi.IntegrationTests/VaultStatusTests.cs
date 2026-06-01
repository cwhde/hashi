using System.Net;
using System.Net.Http.Json;
using Hashi.Contracts.Api;
using Microsoft.AspNetCore.Mvc.Testing;
using Xunit;

namespace Hashi.IntegrationTests;

[Collection(PostgresIntegrationCollection.Name)]
public sealed class VaultStatusTests : IAsyncLifetime
{
    private readonly PostgresIntegrationFixture _fixture;
    private WebApplicationFactory<Program>? _factory;
    private HttpClient? _client;

    public VaultStatusTests(PostgresIntegrationFixture fixture)
    {
        _fixture = fixture;
    }

    public async Task InitializeAsync()
    {
        if (!_fixture.IsAvailable)
        {
            return;
        }

        var connectionString = await _fixture.CreateDatabaseAsync();
        _factory = IntegrationTestApp.CreateFactory(connectionString);
        _client = _factory.CreateClient(IntegrationTestApp.HttpsClientOptions());
        await IntegrationTestAuth.EnsureBootstrapCredentialsAsync(_factory.Services);
        await IntegrationTestAuth.AuthenticateAsBootstrapAsync(_client);
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
    public async Task Vault_status_starts_not_configured()
    {
        if (!_fixture.IsAvailable || _client is null)
        {
            return;
        }

        var status = await _client.GetFromJsonAsync<VaultStatusResponse>("/api/vault/status");
        Assert.NotNull(status);
        Assert.Equal("NotConfigured", status.LockState);
        Assert.False(status.IsVaultConfigured);
        Assert.False(status.HasPasskey);
    }

    [Fact]
    public async Task Recovery_key_generation_returns_formatted_key()
    {
        if (!_fixture.IsAvailable || _client is null)
        {
            return;
        }

        using var request = await IntegrationTestAuth.CreateCsrfRequestAsync(
            _client,
            HttpMethod.Post,
            "/api/vault/recovery-key/generate");

        var response = await _client.SendAsync(request);
        Assert.Equal(HttpStatusCode.OK, response.StatusCode);

        var payload = await response.Content.ReadFromJsonAsync<VaultGenerateRecoveryKeyResponse>();
        Assert.NotNull(payload);
        Assert.Contains('-', payload.RecoveryKey);
        Assert.True(payload.RecoveryKey.Length >= 35);
    }

    [Fact]
    public async Task Passkey_and_vault_setup_steps_require_completion_endpoint()
    {
        if (!_fixture.IsAvailable || _client is null)
        {
            return;
        }

        using var request = await IntegrationTestAuth.CreateCsrfRequestAsync(
            _client,
            HttpMethod.Post,
            "/api/setup/steps/passkey-and-vault/complete");
        var blocked = await _client.SendAsync(request);
        Assert.Equal(HttpStatusCode.BadRequest, blocked.StatusCode);
    }
}
