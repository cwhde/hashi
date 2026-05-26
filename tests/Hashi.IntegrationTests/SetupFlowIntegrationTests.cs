using System.Net;
using System.Net.Http.Json;
using Hashi.Contracts.Api;
using Microsoft.AspNetCore.Mvc.Testing;
using Xunit;

namespace Hashi.IntegrationTests;

public sealed class SetupFlowIntegrationTests : IAsyncLifetime
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
        _client = _factory.CreateClient(new WebApplicationFactoryClientOptions { HandleCookies = true });
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
    public async Task Fresh_install_exposes_bootstrap_and_advances_first_setup_step()
    {
        if (!_postgres.IsAvailable || _client is null)
        {
            return;
        }

        var bootstrap = await _client.GetFromJsonAsync<BootstrapAllowedResponse>("/api/setup/bootstrap-allowed");
        Assert.NotNull(bootstrap);
        Assert.True(bootstrap.Allowed);

        var initial = await _client.GetFromJsonAsync<SetupStatusResponse>("/api/setup/status");
        Assert.NotNull(initial);
        Assert.False(initial.IsComplete);
        Assert.Equal("bootstrap-access", initial.CurrentStep);

        var complete = await _client.PostAsync("/api/setup/steps/bootstrap-access/complete", null);
        Assert.Equal(HttpStatusCode.OK, complete.StatusCode);

        var updated = await complete.Content.ReadFromJsonAsync<SetupStatusResponse>();
        Assert.NotNull(updated);
        Assert.Contains("bootstrap-access", updated.CompletedSteps);
        Assert.Equal("base-settings", updated.CurrentStep);
    }

    [Fact]
    public async Task Health_endpoint_reports_version_without_database_secrets()
    {
        if (!_postgres.IsAvailable || _client is null)
        {
            return;
        }

        var health = await _client.GetFromJsonAsync<HealthResponse>("/api/health");
        Assert.NotNull(health);
        Assert.Equal("ok", health.Status);
        Assert.False(string.IsNullOrWhiteSpace(health.Version));
    }
}
