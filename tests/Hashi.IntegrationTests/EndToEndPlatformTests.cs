using System.Net;
using System.Net.Http.Json;
using Hashi.Contracts.Api;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Microsoft.AspNetCore.Mvc.Testing;
using Microsoft.Extensions.DependencyInjection;
using Xunit;

namespace Hashi.IntegrationTests;

[Collection(PostgresIntegrationCollection.Name)]
public sealed class EndToEndPlatformTests : IAsyncLifetime
{
    private readonly PostgresIntegrationFixture _fixture;
    private WebApplicationFactory<Program>? _factory;
    private HttpClient? _client;

    public EndToEndPlatformTests(PostgresIntegrationFixture fixture)
    {
        _fixture = fixture;
    }

    public async Task InitializeAsync()
    {
        if (!_fixture.IsAvailable)
        {
            return;
        }

        Environment.SetEnvironmentVariable("HASHI_SKIP_STARTUP_HOOKS", "1");
        var connectionString = await _fixture.CreateDatabaseAsync();
        _factory = IntegrationTestApp.CreateFactory(connectionString);
        _client = _factory.CreateClient(new WebApplicationFactoryClientOptions
        {
            HandleCookies = true,
            AllowAutoRedirect = false,
        });
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
    public async Task Setup_advances_and_resource_crud_works()
    {
        if (!_fixture.IsAvailable || _client is null)
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
        if (!_fixture.IsAvailable || _client is null)
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
        if (!_fixture.IsAvailable || _client is null)
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

    [Fact]
    public async Task Public_ports_expose_public_data_without_admin_api_access()
    {
        if (!_fixture.IsAvailable || _factory is null)
        {
            return;
        }

        await using (var scope = _factory.Services.CreateAsyncScope())
        {
            var db = scope.ServiceProvider.GetRequiredService<HashiDbContext>();
            var resource = new ResourceEntity
            {
                Name = "Public App",
                Slug = "public-app",
                Kind = "https",
                Domain = "public.example.com",
                TargetScheme = "http",
                TargetHost = "10.0.0.10",
                TargetPort = 8080,
                DashboardEnabled = true,
                StatusEnabled = true,
            };
            db.Resources.Add(resource);
            db.MonitorEndpoints.Add(new MonitorEndpointEntity
            {
                Name = "Public App",
                Url = "https://public.example.com/",
                CheckType = "https",
                Enabled = true,
                Status = "up",
                ResourceId = resource.Id,
                LastLatencyMs = 42,
                LastCheckedAtUtc = DateTimeOffset.UtcNow,
            });
            await db.SaveChangesAsync();
        }

        using var dashboardClient = _factory.CreateClient(new WebApplicationFactoryClientOptions
        {
            BaseAddress = new Uri("http://localhost:8081"),
            HandleCookies = false,
            AllowAutoRedirect = false,
        });
        using var statusClient = _factory.CreateClient(new WebApplicationFactoryClientOptions
        {
            BaseAddress = new Uri("http://localhost:8082"),
            HandleCookies = false,
            AllowAutoRedirect = false,
        });

        var apps = await dashboardClient.GetFromJsonAsync<IReadOnlyList<ResourceResponse>>("/api/public/apps");
        Assert.NotNull(apps);
        Assert.Contains(apps!, x => x.Name == "Public App" && x.Domain == "public.example.com");

        var statusItems = await statusClient.GetFromJsonAsync<IReadOnlyList<PublicStatusItemResponse>>("/api/public/status");
        Assert.NotNull(statusItems);
        Assert.Contains(statusItems!, x => x.Name == "Public App" && x.Status == "Up" && x.LastLatencyMs == 42);

        var adminFromDashboard = await dashboardClient.GetAsync("/api/resources");
        var adminFromStatus = await statusClient.GetAsync("/api/resources");
        var crossPortStatus = await dashboardClient.GetAsync("/api/public/status");
        Assert.Equal(HttpStatusCode.NotFound, adminFromDashboard.StatusCode);
        Assert.Equal(HttpStatusCode.NotFound, adminFromStatus.StatusCode);
        Assert.Equal(HttpStatusCode.NotFound, crossPortStatus.StatusCode);
    }

    private sealed record CsrfToken(string? Token);
}
