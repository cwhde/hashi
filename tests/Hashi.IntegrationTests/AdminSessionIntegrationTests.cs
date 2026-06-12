using System.Net;
using System.Net.Http.Json;
using Hashi.Contracts.Api;
using Hashi.Infrastructure.Persistence;
using Microsoft.AspNetCore.Mvc.Testing;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Xunit;

namespace Hashi.IntegrationTests;

[Collection(PostgresIntegrationCollection.Name)]
public sealed class AdminSessionIntegrationTests : IAsyncLifetime
{
    private readonly PostgresIntegrationFixture _fixture;
    private WebApplicationFactory<Program>? _factory;

    public AdminSessionIntegrationTests(PostgresIntegrationFixture fixture)
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
        await IntegrationTestAuth.EnsureBootstrapCredentialsAsync(_factory.Services);
    }

    public async Task DisposeAsync()
    {
        if (_factory is not null)
        {
            await _factory.DisposeAsync();
        }
    }

    [Fact]
    public async Task Issued_token_is_rejected_and_revoked_from_another_ip()
    {
        if (_factory is null)
        {
            return;
        }

        using var client = _factory.CreateClient(IntegrationTestApp.HttpsClientOptions());
        var login = await SendBootstrapLoginAsync(client, "203.0.113.10");
        Assert.Equal(HttpStatusCode.OK, login.StatusCode);

        using var validRequest = new HttpRequestMessage(HttpMethod.Get, "/api/auth/session");
        validRequest.Headers.Add("X-Forwarded-For", "203.0.113.10");
        var valid = await client.SendAsync(validRequest);
        Assert.Equal(HttpStatusCode.OK, valid.StatusCode);
        var status = await valid.Content.ReadFromJsonAsync<SessionStatusResponse>();
        Assert.Equal("203.0.113.10", status?.BoundIp);

        using var movedRequest = new HttpRequestMessage(HttpMethod.Get, "/api/auth/session");
        movedRequest.Headers.Add("X-Forwarded-For", "203.0.113.11");
        var moved = await client.SendAsync(movedRequest);
        Assert.Equal(HttpStatusCode.Unauthorized, moved.StatusCode);

        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<HashiDbContext>();
        var session = await db.AdminSessions.SingleAsync();
        Assert.Equal("ip_mismatch", session.RevocationReason);
    }

    [Fact]
    public async Task Separate_logins_from_different_ips_receive_independent_tokens()
    {
        if (_factory is null)
        {
            return;
        }

        using var firstClient = _factory.CreateClient(IntegrationTestApp.HttpsClientOptions());
        using var secondClient = _factory.CreateClient(IntegrationTestApp.HttpsClientOptions());
        Assert.Equal(HttpStatusCode.OK, (await SendBootstrapLoginAsync(firstClient, "203.0.113.10")).StatusCode);
        Assert.Equal(HttpStatusCode.OK, (await SendBootstrapLoginAsync(secondClient, "203.0.113.11")).StatusCode);

        Assert.Equal(HttpStatusCode.OK, (await SendSessionRequestAsync(firstClient, "203.0.113.10")).StatusCode);
        Assert.Equal(HttpStatusCode.OK, (await SendSessionRequestAsync(secondClient, "203.0.113.11")).StatusCode);

        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<HashiDbContext>();
        var boundIps = await db.AdminSessions.OrderBy(x => x.BoundIp).Select(x => x.BoundIp).ToListAsync();
        Assert.Equal(["203.0.113.10", "203.0.113.11"], boundIps);
    }

    private static Task<HttpResponseMessage> SendBootstrapLoginAsync(HttpClient client, string clientIp)
    {
        var request = new HttpRequestMessage(HttpMethod.Post, "/api/auth/bootstrap/login")
        {
            Content = JsonContent.Create(new BootstrapLoginRequest(
                IntegrationTestAuth.Username,
                IntegrationTestAuth.Password)),
        };
        request.Headers.Add("X-Forwarded-For", clientIp);
        return client.SendAsync(request);
    }

    private static Task<HttpResponseMessage> SendSessionRequestAsync(HttpClient client, string clientIp)
    {
        var request = new HttpRequestMessage(HttpMethod.Get, "/api/auth/session");
        request.Headers.Add("X-Forwarded-For", clientIp);
        return client.SendAsync(request);
    }
}
