using System.Net;
using System.Net.Http.Json;
using Hashi.Contracts.Api;
using Microsoft.AspNetCore.Mvc.Testing;
using Xunit;

namespace Hashi.IntegrationTests;

public sealed class SetupPersistenceTests : IAsyncLifetime
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
    public async Task Setup_status_persists_and_resumes_after_step_completion()
    {
        if (!_postgres.IsAvailable || _client is null)
        {
            return;
        }

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

        await using var factory2 = IntegrationTestApp.CreateFactory(_postgres.ConnectionString);
        using var client2 = factory2.CreateClient();
        var resumed = await client2.GetFromJsonAsync<SetupStatusResponse>("/api/setup/status");
        Assert.NotNull(resumed);
        Assert.Contains("bootstrap-access", resumed.CompletedSteps);
        Assert.Equal("base-settings", resumed.CurrentStep);
    }

    [Fact]
    public async Task Audit_log_records_setup_actions()
    {
        if (!_postgres.IsAvailable || _client is null)
        {
            return;
        }

        await _client.PostAsync("/api/setup/steps/bootstrap-access/complete", null);

        var events = await _client.GetFromJsonAsync<List<AuditEventResponse>>("/api/activity/audit");
        Assert.NotNull(events);
        Assert.Contains(events, e => e.Category == "setup" && e.Action == "step_completed");
    }
}
