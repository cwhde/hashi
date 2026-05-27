using System.Net.Http.Json;
using System.Security.Claims;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Services;
using Microsoft.AspNetCore.Authentication.Cookies;
using Microsoft.AspNetCore.Http;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;

namespace Hashi.IntegrationTests;

public static class IntegrationTestAuth
{
    public const string Username = "integration-bootstrap";
    public const string Password = "integration-bootstrap-pass";

    public static async Task EnsureBootstrapCredentialsAsync(IServiceProvider services)
    {
        using var scope = services.CreateScope();
        var setupState = scope.ServiceProvider.GetRequiredService<SetupStateService>();
        var state = await setupState.GetOrCreateAsync();
        state.BootstrapUsername = Username;
        state.BootstrapPasswordHash = BCrypt.Net.BCrypt.HashPassword(Password);
        await scope.ServiceProvider.GetRequiredService<HashiDbContext>().SaveChangesAsync();
    }

    public static async Task AuthenticateAsBootstrapAsync(HttpClient client)
    {
        var csrf = await client.GetFromJsonAsync<CsrfResponse>("/api/auth/csrf");
        var request = new HttpRequestMessage(HttpMethod.Post, "/api/auth/bootstrap/login")
        {
            Content = JsonContent.Create(new { username = Username, password = Password }),
        };
        if (!string.IsNullOrEmpty(csrf?.Token))
        {
            request.Headers.Add("X-CSRF-TOKEN", csrf.Token);
        }

        var response = await client.SendAsync(request);
        response.EnsureSuccessStatusCode();
    }

    public static void MarkBootstrapReauthenticated(IServiceProvider services)
    {
        var identity = new ClaimsIdentity(
            [new Claim(ClaimTypes.Name, "admin")],
            CookieAuthenticationDefaults.AuthenticationScheme);
        var context = new DefaultHttpContext { User = new ClaimsPrincipal(identity) };
        services.GetRequiredService<ReauthenticationState>().MarkRecent(context);
    }

    public static async Task<HttpRequestMessage> CreateCsrfRequestAsync(
        HttpClient client,
        HttpMethod method,
        string requestUri,
        HttpContent? content = null)
    {
        var csrf = await client.GetFromJsonAsync<CsrfResponse>("/api/auth/csrf");
        var request = new HttpRequestMessage(method, requestUri)
        {
            Content = content,
        };
        if (!string.IsNullOrEmpty(csrf?.Token))
        {
            request.Headers.Add("X-CSRF-TOKEN", csrf.Token);
        }

        return request;
    }

    private sealed record CsrfResponse(string? Token);
}
