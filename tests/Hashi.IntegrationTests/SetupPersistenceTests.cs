using System.Net;
using System.Net.Http.Json;
using Hashi.Contracts.Api;
using Microsoft.AspNetCore.Mvc.Testing;
using Testcontainers.PostgreSql;
using Xunit;

namespace Hashi.IntegrationTests;

public sealed class SetupPersistenceTests : IAsyncLifetime
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
    public async Task Setup_status_persists_and_resumes_after_step_completion()
    {
        if (_dockerUnavailable)
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

        // Simulate restart with a fresh client against the same database
        await using var factory2 = new WebApplicationFactory<Program>()
            .WithWebHostBuilder(builder =>
            {
                builder.UseSetting("ConnectionStrings:Hashi", _postgres.GetConnectionString());
            });
        using var client2 = factory2.CreateClient();
        var resumed = await client2.GetFromJsonAsync<SetupStatusResponse>("/api/setup/status");
        Assert.NotNull(resumed);
        Assert.Contains("bootstrap-access", resumed.CompletedSteps);
        Assert.Equal("base-settings", resumed.CurrentStep);
    }

    [Fact]
    public async Task Audit_log_records_setup_actions()
    {
        if (_dockerUnavailable)
        {
            return;
        }

        await _client.PostAsync("/api/setup/steps/bootstrap-access/complete", null);

        var events = await _client.GetFromJsonAsync<List<AuditEventResponse>>("/api/activity/audit");
        Assert.NotNull(events);
        Assert.Contains(events, e => e.Category == "setup" && e.Action == "step_completed");
    }
}
