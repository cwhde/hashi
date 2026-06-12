using System.Net.Http.Json;
using System.Security.Claims;
using Microsoft.AspNetCore.Authentication;
using Microsoft.AspNetCore.Authentication.Cookies;
using Microsoft.AspNetCore.DataProtection;
using Hashi.Core.Auth;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Services;
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

    public static void AuthenticateAsAdminSession(
        HttpClient client,
        IServiceProvider services,
        string sessionId,
        bool unlockVault = false)
    {
        using (var scope = services.CreateScope())
        {
            var db = scope.ServiceProvider.GetRequiredService<HashiDbContext>();
            var now = DateTimeOffset.UtcNow;
            db.AdminSessions.Add(new AdminSessionEntity
            {
                Id = sessionId,
                AuthMethod = AdminAuthMethods.Bootstrap,
                BoundIp = "127.0.0.1",
                ScopesJson = System.Text.Json.JsonSerializer.Serialize(AdminSessionScopes.All),
                IdleTimeoutMinutes = 240,
                CreatedAtUtc = now,
                LastSeenAtUtc = now,
                IdleExpiresAtUtc = now.AddHours(4),
                AbsoluteExpiresAtUtc = now.AddHours(8),
            });
            db.SaveChanges();
        }

        var protector = services.GetRequiredService<IDataProtectionProvider>().CreateProtector(
            "Microsoft.AspNetCore.Authentication.Cookies.CookieAuthenticationMiddleware",
            CookieAuthenticationDefaults.AuthenticationScheme,
            "v2");
        var format = new TicketDataFormat(protector);
        var ticket = new AuthenticationTicket(
            CreateAdminPrincipal(sessionId),
            new AuthenticationProperties
            {
                IssuedUtc = DateTimeOffset.UtcNow,
                ExpiresUtc = DateTimeOffset.UtcNow.AddHours(1),
                IsPersistent = false,
            },
            CookieAuthenticationDefaults.AuthenticationScheme);

        client.DefaultRequestHeaders.Remove("Cookie");
        client.DefaultRequestHeaders.Add("Cookie", $"hashi.session={Uri.EscapeDataString(format.Protect(ticket))}");

        if (unlockVault)
        {
            services.GetRequiredService<VaultSessionState>().UnlockForSession(sessionId, new byte[32]);
        }
    }

    public static DefaultHttpContext CreateAdminHttpContext(string sessionId)
        => new() { User = CreateAdminPrincipal(sessionId) };

    public static void MarkRecentReauthentication(IServiceProvider services, string sessionId = "admin")
    {
        using var scope = services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<HashiDbContext>();
        var session = db.AdminSessions.Single(x => x.Id == sessionId);
        session.ReauthenticatedAtUtc = DateTimeOffset.UtcNow;
        db.SaveChanges();
    }

    private static ClaimsPrincipal CreateAdminPrincipal(string sessionId)
    {
        var identity = new ClaimsIdentity(
            [
                new Claim(ClaimTypes.NameIdentifier, sessionId),
                new Claim(ClaimTypes.Sid, sessionId),
                new Claim(ClaimTypes.Name, "admin"),
                new Claim(AdminClaimTypes.AuthMethod, AdminAuthMethods.Bootstrap),
            ],
            CookieAuthenticationDefaults.AuthenticationScheme);
        return new ClaimsPrincipal(identity);
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
