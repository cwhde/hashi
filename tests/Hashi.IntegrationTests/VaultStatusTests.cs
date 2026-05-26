using System.Net;
using System.Net.Http.Json;
using Hashi.Contracts.Api;
using Microsoft.AspNetCore.Mvc.Testing;
using Xunit;

namespace Hashi.IntegrationTests;

public sealed class VaultStatusTests : IAsyncLifetime
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

        _factory = IntegrationTestApp.CreateFactory(_postgres.ConnectionString);
        _client = _factory.CreateClient();
        await IntegrationTestApp.MigrateAsync(_factory.Services);
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
    public async Task Vault_status_starts_not_configured()
    {
        if (!_postgres.IsAvailable || _client is null)
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
        if (!_postgres.IsAvailable || _client is null)
        {
            return;
        }

        var csrf = await _client.GetFromJsonAsync<CsrfToken>("/api/auth/csrf");
        var request = new HttpRequestMessage(HttpMethod.Post, "/api/vault/recovery-key/generate");
        if (!string.IsNullOrEmpty(csrf?.Token))
        {
            request.Headers.Add("X-CSRF-TOKEN", csrf.Token);
        }

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
        if (!_postgres.IsAvailable || _client is null)
        {
            return;
        }

        var blocked = await _client.PostAsync("/api/setup/steps/passkey-and-vault/complete", null);
        Assert.Equal(HttpStatusCode.BadRequest, blocked.StatusCode);
    }

    private sealed record CsrfToken(string? Token);
}
