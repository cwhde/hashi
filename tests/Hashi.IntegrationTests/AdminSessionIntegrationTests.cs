using System.Net;
using System.Net.Http.Json;
using Hashi.Contracts.Api;
using Hashi.Core.Auth;
using Hashi.Infrastructure.Auth;
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
    private string? _connectionString;
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

        _connectionString = await _fixture.CreateDatabaseAsync();
        _factory = IntegrationTestApp.CreateFactory(_connectionString);
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

    [Fact]
    public async Task Malformed_forwarded_address_cannot_issue_session()
    {
        if (_factory is null)
        {
            return;
        }

        using var client = _factory.CreateClient(IntegrationTestApp.HttpsClientOptions());
        var response = await SendBootstrapLoginAsync(client, "not-an-ip");

        Assert.Equal(HttpStatusCode.Unauthorized, response.StatusCode);
        using var scope = _factory.Services.CreateScope();
        Assert.Empty(await scope.ServiceProvider.GetRequiredService<HashiDbContext>().AdminSessions.ToListAsync());
    }

    [Fact]
    public async Task Malformed_forwarded_address_revokes_existing_session()
    {
        if (_factory is null)
        {
            return;
        }

        using var client = _factory.CreateClient(IntegrationTestApp.HttpsClientOptions());
        Assert.Equal(HttpStatusCode.OK, (await SendBootstrapLoginAsync(client, "203.0.113.10")).StatusCode);

        var malformedRequest = new HttpRequestMessage(HttpMethod.Get, "/api/auth/session");
        malformedRequest.Headers.Add("X-Forwarded-For", "not-an-ip");
        var response = await client.SendAsync(malformedRequest);

        Assert.Equal(HttpStatusCode.Unauthorized, response.StatusCode);
        using var scope = _factory.Services.CreateScope();
        var session = await scope.ServiceProvider.GetRequiredService<HashiDbContext>().AdminSessions.SingleAsync();
        Assert.Equal("client_ip_unavailable", session.RevocationReason);
    }

    [Fact]
    public async Task Revoked_session_is_rejected_before_cookie_expiry()
    {
        if (_factory is null)
        {
            return;
        }

        using var client = _factory.CreateClient(IntegrationTestApp.HttpsClientOptions());
        const string sessionId = "revocation-integration-session";
        IntegrationTestAuth.AuthenticateAsAdminSession(client, _factory.Services, sessionId);

        using (var scope = _factory.Services.CreateScope())
        {
            await scope.ServiceProvider.GetRequiredService<AdminSessionService>()
                .RevokeAsync(sessionId, "manual");
        }

        var response = await client.GetAsync("/api/auth/session");

        Assert.Equal(HttpStatusCode.Unauthorized, response.StatusCode);
    }

    [Fact]
    public async Task Session_and_recent_reauthentication_survive_application_restart()
    {
        if (_factory is null || _connectionString is null)
        {
            return;
        }

        string sessionId;
        using (var scope = _factory.Services.CreateScope())
        {
            var sessions = scope.ServiceProvider.GetRequiredService<AdminSessionService>();
            var session = await sessions.CreateAsync(
                AdminAuthMethods.Passkey,
                "203.0.113.10",
                AdminSessionScopes.All);
            sessionId = session.Id;
            await sessions.MarkReauthenticatedAsync(sessionId);
        }

        await _factory.DisposeAsync();
        _factory = IntegrationTestApp.CreateFactory(_connectionString);

        using var restartedScope = _factory.Services.CreateScope();
        var restartedSessions = restartedScope.ServiceProvider.GetRequiredService<AdminSessionService>();
        var validation = await restartedSessions.ValidateAsync(sessionId, "203.0.113.10");

        Assert.True(validation.IsValid);
        Assert.True(await restartedSessions.IsRecentlyReauthenticatedAsync(sessionId));
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
