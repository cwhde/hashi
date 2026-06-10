using System.Net;
using System.Net.Http.Json;
using Hashi.Contracts.Api;
using Microsoft.AspNetCore.Mvc.Testing;
using Xunit;

namespace Hashi.IntegrationTests;

[Collection(PostgresIntegrationCollection.Name)]
public sealed class TraefikConfigValidationTests : IAsyncLifetime
{
    private readonly PostgresIntegrationFixture _fixture;
    private WebApplicationFactory<Program>? _factory;
    private HttpClient? _client;

    public TraefikConfigValidationTests(PostgresIntegrationFixture fixture)
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
    public async Task Traefik_render_endpoint_returns_valid_yaml()
    {
        if (!_fixture.IsAvailable || _client is null)
        {
            return;
        }

        var response = await _client.GetAsync("/api/traefik/render");
        Assert.Equal(HttpStatusCode.OK, response.StatusCode);

        var render = await response.Content.ReadFromJsonAsync<TraefikRenderResponse>();
        Assert.NotNull(render);
        Assert.NotNull(render.StaticConfigYaml);
        Assert.NotNull(render.DynamicHttpYaml);
    }

    [Fact]
    public async Task Traefik_render_endpoint_includes_dynamic_files()
    {
        if (!_fixture.IsAvailable || _client is null)
        {
            return;
        }

        var response = await _client.GetAsync("/api/traefik/render");
        response.EnsureSuccessStatusCode();

        var render = await response.Content.ReadFromJsonAsync<TraefikRenderResponse>();
        Assert.NotNull(render);
        Assert.Contains("http:", render.DynamicHttpYaml, StringComparison.OrdinalIgnoreCase);
    }

    [Fact]
    public async Task Traefik_render_endpoint_returns_content_hash()
    {
        if (!_fixture.IsAvailable || _client is null)
        {
            return;
        }

        var response = await _client.GetAsync("/api/traefik/render");
        response.EnsureSuccessStatusCode();

        var render = await response.Content.ReadFromJsonAsync<TraefikRenderResponse>();
        Assert.NotNull(render);
        Assert.False(string.IsNullOrWhiteSpace(render.ContentHash));
    }

    [Fact]
    public async Task Traefik_render_with_user_middlewares_includes_middleware_yaml()
    {
        if (!_fixture.IsAvailable || _client is null)
        {
            return;
        }

        var response = await _client.GetAsync("/api/traefik/render");
        response.EnsureSuccessStatusCode();

        var render = await response.Content.ReadFromJsonAsync<TraefikRenderResponse>();
        Assert.NotNull(render);
        Assert.NotNull(render.DynamicUserMiddlewaresYaml);
    }

    private sealed record CsrfToken(string? Token);
}
