using System.Net;
using System.Net.Http.Json;
using Hashi.IntegrationTests.Fakes;
using Microsoft.AspNetCore.Mvc.Testing;
using Xunit;

namespace Hashi.IntegrationTests;

[Collection(PostgresIntegrationCollection.Name)]
public sealed class AdGuardIntegrationTests : IAsyncLifetime
{
    private readonly PostgresIntegrationFixture _fixture;
    private readonly FakeAdGuardHandler _fake = new();
    private WebApplicationFactory<Program>? _factory;
    private HttpClient? _client;

    public AdGuardIntegrationTests(PostgresIntegrationFixture fixture)
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
        _factory = IntegrationTestApp.CreateFactory(connectionString)
            .WithWebHostBuilder(builder =>
            {
                builder.ConfigureServices(services =>
                {
                    services.AddHttpClient("adguard")
                        .ConfigurePrimaryHttpMessageHandler(() => _fake);
                });
            });
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
    public async Task AdGuard_connection_endpoint_accepts_valid_payload()
    {
        if (!_fixture.IsAvailable || _client is null)
        {
            return;
        }

        var csrf = await _client.GetFromJsonAsync<CsrfToken>("/api/auth/csrf");
        var request = new HttpRequestMessage(HttpMethod.Post, "/api/connections/adguard")
        {
            Content = JsonContent.Create(new
            {
                name = "Home DNS",
                baseUrl = "http://adguard.test",
                password = "test-password",
            }),
        };
        if (!string.IsNullOrEmpty(csrf?.Token))
        {
            request.Headers.Add("X-CSRF-TOKEN", csrf.Token);
        }

        var response = await _client.SendAsync(request);
        Assert.Equal(HttpStatusCode.OK, response.StatusCode);
    }

    [Fact]
    public async Task AdGuard_connection_list_endpoint_returns_connections()
    {
        if (!_fixture.IsAvailable || _client is null)
        {
            return;
        }

        var response = await _client.GetAsync("/api/connections");
        Assert.Equal(HttpStatusCode.OK, response.StatusCode);
    }

    private sealed record CsrfToken(string? Token);
}
