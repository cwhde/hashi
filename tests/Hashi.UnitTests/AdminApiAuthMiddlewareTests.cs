using Hashi.Api.Hosting;
using Hashi.Core.Hosting;
using Hashi.Core.Auth;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Services;
using Microsoft.AspNetCore.Http;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging.Abstractions;
using System.Security.Claims;
using Xunit;

namespace Hashi.UnitTests;

public sealed class AdminApiAuthMiddlewareTests
{
    [Theory]
    [InlineData("/api/vault/secrets/abc", "POST", true)]
    [InlineData("/api/vault/secrets/11111111-1111-1111-1111-111111111111/reveal", "GET", true)]
    [InlineData("/api/scripts", "POST", true)]
    [InlineData("/api/scripts/abc/run", "POST", true)]
    [InlineData("/api/connections/ssh", "POST", true)]
    [InlineData("/api/dns/connections/hetzner", "POST", true)]
    [InlineData("/api/settings/general", "PUT", true)]
    [InlineData("/api/sync/apply", "POST", true)]
    [InlineData("/api/firewall/apply", "POST", true)]
    [InlineData("/api/traefik/apply", "POST", true)]
    [InlineData("/api/resources/abc", "DELETE", true)]
    [InlineData("/api/security/blocklist/sync", "POST", true)]
    [InlineData("/api/security/manual-entries", "POST", true)]
    [InlineData("/api/security/manual-entries/11111111-1111-1111-1111-111111111111", "PATCH", true)]
    [InlineData("/api/security/blocks", "POST", true)]
    [InlineData("/api/security/blocks/11111111-1111-1111-1111-111111111111/make-permanent", "POST", true)]
    [InlineData("/api/security/blocks/11111111-1111-1111-1111-111111111111/preview-firewall-sync", "POST", true)]
    [InlineData("/api/auth/sessions/revoke-others", "POST", true)]
    [InlineData("/api/auth/sessions/0123456789abcdef", "DELETE", true)]
    [InlineData("/api/auth/passkeys/11111111-1111-1111-1111-111111111111", "DELETE", true)]
    [InlineData("/api/pulse/agents", "POST", true)]
    [InlineData("/api/resources", "GET", false)]
    [InlineData("/api/resources", "POST", false)]
    [InlineData("/api/sync/runs", "GET", false)]
    public void RequiresReauthentication_matches_spec_paths(string path, string method, bool expected)
    {
        var actual = AdminApiAuthMiddleware.RequiresReauthentication(new PathString(path), method);
        Assert.Equal(expected, actual);
    }

    [Theory]
    [InlineData("/api/resources", "GET", AdminSessionScopes.Read)]
    [InlineData("/api/resources", "POST", AdminSessionScopes.Write)]
    [InlineData("/api/settings/admin-session", "PUT", AdminSessionScopes.SettingsManage)]
    [InlineData("/api/vault/secrets", "POST", AdminSessionScopes.SecretsManage)]
    [InlineData("/api/scripts/abc/run", "POST", AdminSessionScopes.ScriptsManage)]
    [InlineData("/api/security/blocks", "POST", AdminSessionScopes.SecurityManage)]
    [InlineData("/api/auth/sessions/revoke-others", "POST", AdminSessionScopes.SecurityManage)]
    [InlineData("/api/firewall/abc/apply", "POST", AdminSessionScopes.FirewallApply)]
    [InlineData("/api/sync/abc/apply", "POST", AdminSessionScopes.SyncApply)]
    public void RequiredScope_maps_admin_operations(string path, string method, string expected)
    {
        Assert.Equal(expected, AdminApiAuthMiddleware.RequiredScope(new PathString(path), method));
    }

    [Theory]
    [InlineData("/api/health", "GET")]
    [InlineData("/api/setup/status", "GET")]
    [InlineData("/api/setup/bootstrap-allowed", "GET")]
    [InlineData("/api/auth/csrf", "GET")]
    [InlineData("/api/auth/bootstrap/login", "POST")]
    [InlineData("/api/auth/passkeys/login/begin", "POST")]
    [InlineData("/api/auth/passkeys/login/complete", "POST")]
    [InlineData("/api/edge-auth/check", "GET")]
    [InlineData("/api/public/status", "GET")]
    public void Public_allowlist_keeps_only_expected_public_endpoints(string path, string method)
    {
        Assert.True(AdminApiAuthMiddleware.IsPublicEndpoint(new PathString(path), method));
    }

    [Theory]
    [InlineData("/api/auth/passkeys", "GET")]
    [InlineData("/api/auth/passkeys/11111111-1111-1111-1111-111111111111", "DELETE")]
    [InlineData("/api/auth/passkeys/register/begin", "POST")]
    [InlineData("/api/auth/passkeys/register/complete", "POST")]
    [InlineData("/api/auth/reauthenticate", "POST")]
    [InlineData("/api/auth/reauthenticate/complete", "POST")]
    [InlineData("/api/auth/logout", "POST")]
    [InlineData("/api/vault/status", "GET")]
    [InlineData("/api/setup/steps/bootstrap-access/complete", "POST")]
    public void Public_allowlist_does_not_include_protected_auth_or_setup_endpoints(string path, string method)
    {
        Assert.False(AdminApiAuthMiddleware.IsPublicEndpoint(new PathString(path), method));
    }

    [Theory]
    [InlineData("/api/auth/passkeys", "GET")]
    [InlineData("/api/auth/passkeys/11111111-1111-1111-1111-111111111111", "DELETE")]
    [InlineData("/api/auth/reauthenticate", "POST")]
    [InlineData("/api/vault/secrets", "POST")]
    [InlineData("/api/settings/general", "GET")]
    [InlineData("/api/activity/audit", "GET")]
    [InlineData("/api/security/access-log", "POST")]
    [InlineData("/api/security/waf-events", "POST")]
    [InlineData("/api/setup/steps/bootstrap-access/complete", "POST")]
    public async Task Anonymous_protected_endpoints_are_rejected_even_before_setup_completes(string path, string method)
    {
        var (context, invoked) = await InvokeMiddlewareAsync(path, method, setupComplete: false);

        Assert.False(invoked);
        Assert.Equal(StatusCodes.Status401Unauthorized, context.Response.StatusCode);
    }

    [Theory]
    [InlineData("/api/setup/status", "GET")]
    [InlineData("/api/pulse/agent-1/heartbeat", "POST")]
    public async Task Anonymous_operational_public_endpoints_still_bypass_auth(string path, string method)
    {
        var (context, invoked) = await InvokeMiddlewareAsync(path, method, setupComplete: false);

        Assert.True(invoked);
        Assert.Equal(StatusCodes.Status204NoContent, context.Response.StatusCode);
    }

    [Fact]
    public async Task Access_log_ingest_allows_authenticated_admin_pipeline()
    {
        var (context, invoked) = await InvokeMiddlewareAsync(
            "/api/security/access-log",
            HttpMethods.Post,
            setupComplete: true,
            user: AuthenticatedUser());

        Assert.True(invoked);
        Assert.Equal(StatusCodes.Status204NoContent, context.Response.StatusCode);
    }

    [Fact]
    public async Task Reauthenticate_begin_requires_auth_but_not_recent_reauthentication()
    {
        var (context, invoked) = await InvokeMiddlewareAsync(
            "/api/auth/reauthenticate",
            HttpMethods.Post,
            setupComplete: true,
            user: AuthenticatedUser());

        Assert.True(invoked);
        Assert.Equal(StatusCodes.Status204NoContent, context.Response.StatusCode);
    }

    [Fact]
    public async Task Secret_reveal_get_requires_recent_reauthentication()
    {
        var (context, invoked) = await InvokeMiddlewareAsync(
            "/api/vault/secrets/11111111-1111-1111-1111-111111111111/reveal",
            HttpMethods.Get,
            setupComplete: true,
            user: AuthenticatedUser());

        Assert.False(invoked);
        Assert.Equal(StatusCodes.Status403Forbidden, context.Response.StatusCode);
    }

    [Fact]
    public async Task Secret_reveal_get_allows_recent_reauthentication()
    {
        var user = AuthenticatedUser();

        var (context, invoked) = await InvokeMiddlewareAsync(
            "/api/vault/secrets/11111111-1111-1111-1111-111111111111/reveal",
            HttpMethods.Get,
            setupComplete: true,
            user: user,
            recentlyReauthenticated: true);

        Assert.True(invoked);
        Assert.Equal(StatusCodes.Status204NoContent, context.Response.StatusCode);
    }

    [Fact]
    public async Task Authenticated_session_without_required_scope_is_rejected()
    {
        var (context, invoked) = await InvokeMiddlewareAsync(
            "/api/settings/admin-session",
            HttpMethods.Put,
            setupComplete: true,
            user: AuthenticatedUser(),
            scopes: [AdminSessionScopes.Read]);

        Assert.False(invoked);
        Assert.Equal(StatusCodes.Status403Forbidden, context.Response.StatusCode);
    }

    [Theory]
    [InlineData("/api/auth/bootstrap/login", "POST", true)]
    [InlineData("/api/auth/passkeys/login/begin", "POST", true)]
    [InlineData("/api/auth/passkeys/login/complete", "POST", true)]
    [InlineData("/api/auth/passkeys", "DELETE", false)]
    [InlineData("/api/auth/reauthenticate", "POST", false)]
    [InlineData("/api/auth/logout", "POST", false)]
    [InlineData("/api/setup/complete", "POST", false)]
    public void Csrf_exemptions_are_limited_to_public_login_ceremonies(string path, string method, bool expected)
    {
        Assert.Equal(expected, AdminCsrfMiddleware.IsCsrfExemptEndpoint(new PathString(path), method));
    }

    private static async Task<(DefaultHttpContext Context, bool Invoked)> InvokeMiddlewareAsync(
        string path,
        string method,
        bool setupComplete,
        ClaimsPrincipal? user = null,
        bool recentlyReauthenticated = false,
        IReadOnlyList<string>? scopes = null)
    {
        await using var db = CreateDb();
        var setup = new SetupStateService(db, NullLogger<SetupStateService>.Instance);
        var state = await setup.GetOrCreateAsync();
        state.IsComplete = setupComplete;
        await db.SaveChangesAsync();

        var context = new DefaultHttpContext
        {
            User = user ?? new ClaimsPrincipal(new ClaimsIdentity()),
        };
        context.Request.Path = path;
        context.Request.Method = method;

        if (context.User.Identity?.IsAuthenticated == true)
        {
            var session = new Hashi.Infrastructure.Persistence.Entities.AdminSessionEntity
            {
                Id = "test-session",
                AuthMethod = AdminAuthMethods.Passkey,
                BoundIp = "127.0.0.1",
                ScopesJson = System.Text.Json.JsonSerializer.Serialize(AdminSessionScopes.All),
                CreatedAtUtc = DateTimeOffset.UtcNow,
                LastSeenAtUtc = DateTimeOffset.UtcNow,
                IdleExpiresAtUtc = DateTimeOffset.UtcNow.AddHours(4),
                AbsoluteExpiresAtUtc = DateTimeOffset.UtcNow.AddHours(8),
                ReauthenticatedAtUtc = recentlyReauthenticated ? DateTimeOffset.UtcNow : null,
            };
            context.Items[AdminSessionCookieEvents.ValidationItemKey] = AdminSessionValidationResult.Valid(
                session,
                scopes ?? AdminSessionScopes.All);
        }

        var invoked = false;
        var middleware = new AdminApiAuthMiddleware(httpContext =>
        {
            invoked = true;
            httpContext.Response.StatusCode = StatusCodes.Status204NoContent;
            return Task.CompletedTask;
        });

        await middleware.InvokeAsync(context, setup);
        return (context, invoked);
    }

    private static ClaimsPrincipal AuthenticatedUser(string authMethod = AdminAuthMethods.Passkey)
    {
        var identity = new ClaimsIdentity(
            [
                new Claim(ClaimTypes.Name, "admin"),
                new Claim(AdminClaimTypes.AuthMethod, authMethod),
            ],
            authenticationType: "Test");
        return new ClaimsPrincipal(identity);
    }

    private static HashiDbContext CreateDb()
    {
        var options = new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        return new HashiDbContext(options);
    }
}

public sealed class DnsDesiredStateBuilderTests
{
    [Fact]
    public void MergeRecords_keeps_generated_conflict_visible_for_manual_record_with_same_name()
    {
        var manual = new[]
        {
            new Hashi.Core.Dns.DnsRecordSnapshot("", "app.example.com", Hashi.Core.Dns.DnsRecordType.A, "1.2.3.4", 3600, true),
        };
        var generated = new[]
        {
            new Hashi.Core.Dns.DnsRecordSnapshot("", "app.example.com", Hashi.Core.Dns.DnsRecordType.A, "203.0.113.10", 3600, true),
        };

        var merged = Hashi.Infrastructure.Dns.DnsDesiredStateBuilder.MergeRecords(manual, generated);

        Assert.Equal(2, merged.Count);
        Assert.Contains(merged, x => x.Value == "1.2.3.4");
        Assert.Contains(merged, x => x.Value == "203.0.113.10");
        Assert.All(merged, x => Assert.True(x.IsManagedByHashi));
    }

    [Fact]
    public async Task BuildAsync_uses_resource_domain_and_does_not_publish_internal_firewall_ip()
    {
        await using var db = CreateDb();
        var zoneId = Guid.NewGuid();
        var pulseAgentId = Guid.NewGuid();
        db.AppSettings.Add(new Hashi.Infrastructure.Persistence.Entities.AppSettingsEntity { RootDomain = "example.com" });
        db.FirewallHosts.Add(new Hashi.Infrastructure.Persistence.Entities.FirewallHostEntity
        {
            ConnectionId = Guid.NewGuid(),
            Name = "edge",
            Domain = "edge.example.com",
            InternalTraefikIp = "10.0.0.2",
            ManagedSubnetsJson = "[]",
        });
        db.PulseAgents.Add(new Hashi.Infrastructure.Persistence.Entities.PulseAgentEntity
        {
            Id = pulseAgentId,
            Name = "pulse-1",
            TokenHash = "hash",
            LastPublicIp = "203.0.113.20",
        });
        db.Resources.Add(new Hashi.Infrastructure.Persistence.Entities.ResourceEntity
        {
            Name = "Custom",
            Slug = "custom",
            Domain = "service.custom.test",
            Enabled = true,
            PulseAgentId = pulseAgentId,
        });
        await db.SaveChangesAsync();

        var records = await Hashi.Infrastructure.Dns.DnsDesiredStateBuilder.BuildAsync(db, zoneId, 3600);

        Assert.Contains(records, x => x.Name == "service.custom.test" && x.Value == "203.0.113.20");
        Assert.DoesNotContain(records, x => x.Name == "custom.example.com");
        Assert.DoesNotContain(records, x => x.Value == "10.0.0.2");
        Assert.DoesNotContain(records, x => x.Name == "edge.example.com");
    }

    private static HashiDbContext CreateDb()
    {
        var options = new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        return new HashiDbContext(options);
    }
}
