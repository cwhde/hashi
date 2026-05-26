using System.Net;
using System.Net.Http.Json;
using Hashi.Contracts.Api;
using Microsoft.AspNetCore.Mvc.Testing;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Xunit;

namespace Hashi.IntegrationTests;

public sealed class EndToEndPlatformTests : IAsyncLifetime
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

        await _postgres.DisposeAsync();
    }

    [Fact]
    public async Task Setup_advances_and_resource_crud_works()
    {
        if (!_postgres.IsAvailable || _client is null)
        {
            return;
        }

        var setup = await _client.GetFromJsonAsync<SetupStatusResponse>("/api/setup/status");
        Assert.NotNull(setup);
        Assert.False(setup.IsComplete);

        var csrf = await _client.GetFromJsonAsync<CsrfToken>("/api/auth/csrf");
        var create = new HttpRequestMessage(HttpMethod.Post, "/api/resources")
        {
            Content = JsonContent.Create(new CreateResourceRequest(
                "Test App", "https", "app.example.com", "http", "127.0.0.1", 8080, true, true)),
        };
        if (!string.IsNullOrEmpty(csrf?.Token))
        {
            create.Headers.Add("X-CSRF-TOKEN", csrf.Token);
        }

        var resource = await _client.SendAsync(create);
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
        if (!_postgres.IsAvailable || _client is null)
        {
            return;
        }

        var dashboard = await _client.GetFromJsonAsync<SecurityDashboardResponse>("/api/security/dashboard");
        Assert.NotNull(dashboard);
        Assert.True(dashboard.Allowed >= 0);
    }

    [Fact]
    public async Task Sync_plan_endpoint_returns_preview()
    {
        if (!_postgres.IsAvailable || _client is null)
        {
            return;
        }

        var csrf = await _client.GetFromJsonAsync<CsrfToken>("/api/auth/csrf");
        var request = new HttpRequestMessage(HttpMethod.Post, "/api/sync/plan");
        if (!string.IsNullOrEmpty(csrf?.Token))
        {
            request.Headers.Add("X-CSRF-TOKEN", csrf.Token);
        }

        var response = await _client.SendAsync(request);
        Assert.Equal(HttpStatusCode.OK, response.StatusCode);
    }

    private sealed record CsrfToken(string? Token);
}
