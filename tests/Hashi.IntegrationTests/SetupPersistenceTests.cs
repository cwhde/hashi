using System.Net;
using System.Net.Http.Json;
using Hashi.Contracts.Api;
using Microsoft.AspNetCore.Mvc.Testing;
using Xunit;

namespace Hashi.IntegrationTests;

[Collection(PostgresIntegrationCollection.Name)]
public sealed class SetupPersistenceTests : IAsyncLifetime
{
    private readonly PostgresIntegrationFixture _fixture;
    private string? _connectionString;
    private WebApplicationFactory<Program>? _factory;
    private HttpClient? _client;

    public SetupPersistenceTests(PostgresIntegrationFixture fixture)
    {
        _fixture = fixture;
    }

    public async Task InitializeAsync()
    {
        if (!_fixture.IsAvailable)
        {
            return;
        }

        _connectionString = await _fixture.CreateDatabaseAsync();
        _factory = IntegrationTestApp.CreateFactory(_connectionString);
        _client = _factory.CreateClient();
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
    public async Task Setup_status_persists_and_resumes_after_step_completion()
    {
        if (!_fixture.IsAvailable || _client is null || _connectionString is null)
        {
            return;
        }

        var initial = await _client.GetFromJsonAsync<SetupStatusResponse>("/api/setup/status");
        Assert.NotNull(initial);
        Assert.False(initial.IsComplete);
        Assert.Equal("bootstrap-access", initial.CurrentStep);

        await IntegrationTestAuth.EnsureBootstrapCredentialsAsync(_factory!.Services);
        await IntegrationTestAuth.AuthenticateAsBootstrapAsync(_client);
        using var completeRequest = await IntegrationTestAuth.CreateCsrfRequestAsync(
            _client,
            HttpMethod.Post,
            "/api/setup/steps/bootstrap-access/complete");
        var complete = await _client.SendAsync(completeRequest);
        Assert.Equal(HttpStatusCode.OK, complete.StatusCode);

        var updated = await complete.Content.ReadFromJsonAsync<SetupStatusResponse>();
        Assert.NotNull(updated);
        Assert.Contains("bootstrap-access", updated.CompletedSteps);
        Assert.Equal("base-settings", updated.CurrentStep);

        await using var factory2 = IntegrationTestApp.CreateFactory(_connectionString);
        using var client2 = factory2.CreateClient();
        var resumed = await client2.GetFromJsonAsync<SetupStatusResponse>("/api/setup/status");
        Assert.NotNull(resumed);
        Assert.Contains("bootstrap-access", resumed.CompletedSteps);
        Assert.Equal("base-settings", resumed.CurrentStep);
    }

    [Fact]
    public async Task Audit_log_records_setup_actions()
    {
        if (!_fixture.IsAvailable || _client is null)
        {
            return;
        }

        await IntegrationTestAuth.EnsureBootstrapCredentialsAsync(_factory!.Services);
        await IntegrationTestAuth.AuthenticateAsBootstrapAsync(_client);
        using var completeRequest = await IntegrationTestAuth.CreateCsrfRequestAsync(
            _client,
            HttpMethod.Post,
            "/api/setup/steps/bootstrap-access/complete");
        var complete = await _client.SendAsync(completeRequest);
        complete.EnsureSuccessStatusCode();

        var events = await _client.GetFromJsonAsync<List<AuditEventResponse>>("/api/activity/audit");
        Assert.NotNull(events);
        Assert.Contains(events, e => e.Category == "setup" && e.Action == "step_completed");
    }
}
