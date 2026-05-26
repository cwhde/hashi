using System.Net;
using System.Net.Http.Json;
using Hashi.Contracts.Api;
using Microsoft.AspNetCore.Mvc.Testing;
using Testcontainers.PostgreSql;
using Xunit;

namespace Hashi.IntegrationTests;

public sealed class EndToEndPlatformTests : IAsyncLifetime
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
    public async Task Setup_advances_and_resource_crud_works()
    {
        var setup = await _client.GetFromJsonAsync<SetupStatusResponse>("/api/setup/status");
        Assert.NotNull(setup);
        Assert.False(setup.IsComplete);

        var resource = await _client.PostAsJsonAsync("/api/resources", new CreateResourceRequest(
            "Test App", "https", "app.example.com", "http", "127.0.0.1", 8080, true, true));
        Assert.Equal(HttpStatusCode.OK, resource.StatusCode);
        var created = await resource.Content.ReadFromJsonAsync<ResourceResponse>();
        Assert.NotNull(created);
        Assert.Equal("test-app", created.Slug);

        var render = await _client.GetFromJsonAsync<TraefikRenderResponse>("/api/traefik/render");
        Assert.NotNull(render);
        Assert.Contains("app.example.com", render.DynamicHttpYaml);
    }

    [Fact]
    public async Task Security_dashboard_returns_metrics_shape()
    {
        var dashboard = await _client.GetFromJsonAsync<SecurityDashboardResponse>("/api/security/dashboard");
        Assert.NotNull(dashboard);
        Assert.True(dashboard.Allowed >= 0);
    }
}
