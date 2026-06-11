using System.Net;
using System.Net.Http.Json;
using System.Text.Json;
using Hashi.Contracts.Api;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Microsoft.AspNetCore.Mvc.Testing;
using Microsoft.EntityFrameworkCore;
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
        _client = _factory.CreateClient(IntegrationTestApp.HttpsClientOptions(allowAutoRedirect: false));
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
    public async Task Admin_login_and_csrf_cookies_are_secure()
    {
        if (!_fixture.IsAvailable || _factory is null)
        {
            return;
        }

        using var client = _factory.CreateClient(new WebApplicationFactoryClientOptions
        {
            BaseAddress = new Uri("https://localhost"),
            HandleCookies = false,
            AllowAutoRedirect = false,
        });

        var csrf = await client.GetAsync("/api/auth/csrf");
        csrf.EnsureSuccessStatusCode();
        AssertSetCookieFlag(csrf, "hashi.csrf", "Secure");
        AssertSetCookieFlag(csrf, "hashi.csrf", "HttpOnly");

        var login = await client.PostAsJsonAsync("/api/auth/bootstrap/login", new
        {
            username = IntegrationTestAuth.Username,
            password = IntegrationTestAuth.Password,
        });
        login.EnsureSuccessStatusCode();
        AssertSetCookieFlag(login, "hashi.session", "Secure");
        AssertSetCookieFlag(login, "hashi.session", "HttpOnly");
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
    public async Task Admin_dashboard_endpoint_returns_aggregated_overview_data()
    {
        if (!_fixture.IsAvailable || _client is null)
        {
            return;
        }

        var response = await _client.GetAsync("/api/dashboard");
        Assert.Equal(HttpStatusCode.OK, response.StatusCode);
        
        var dashboard = await response.Content.ReadFromJsonAsync<AdminDashboardResponse>();
        Assert.NotNull(dashboard);
        Assert.NotNull(dashboard.Health);
        Assert.NotNull(dashboard.Vault);
        Assert.NotNull(dashboard.AuditEvents);
        Assert.NotNull(dashboard.Resources);
        Assert.NotNull(dashboard.Monitors);
        Assert.NotNull(dashboard.Security);
        Assert.NotNull(dashboard.DnsConnections);
        Assert.NotNull(dashboard.PulseAgents);
        Assert.NotNull(dashboard.SyncRuns);
    }

    [Fact]
    public async Task Waf_event_ingest_endpoint_records_security_event()
    {
        if (!_fixture.IsAvailable || _client is null || _factory is null)
        {
            return;
        }

        var request = await IntegrationTestAuth.CreateCsrfRequestAsync(
            _client,
            HttpMethod.Post,
            "/api/security/waf-events",
            JsonContent.Create(new WafEventIngestRequest("203.0.113.10", "app.example.com", "/admin", "deny")));

        var response = await _client.SendAsync(request);

        Assert.Equal(HttpStatusCode.OK, response.StatusCode);
        await using var scope = _factory.Services.CreateAsyncScope();
        var db = scope.ServiceProvider.GetRequiredService<HashiDbContext>();
        var stored = await db.SecurityEvents.SingleAsync(x => x.Category == "waf");
        Assert.Equal("blocked", stored.Action);
        Assert.Equal("203.0.113.10", stored.ClientIp);
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
                PublicStatusEnabled = true,
                Status = "up",
                ResourceId = resource.Id,
                LastLatencyMs = 42,
                LastCheckedAtUtc = DateTimeOffset.UtcNow,
            });
            db.MonitorEndpoints.Add(new MonitorEndpointEntity
            {
                Name = "Private App",
                Url = "https://private.example.com/",
                CheckType = "https",
                Enabled = true,
                PublicStatusEnabled = false,
                Status = "up",
            });
            var dnsConnection = new ConnectionEntity
            {
                Name = "dns",
                Type = ConnectionTypeNames.DnsProvider,
            };
            var zone = new DnsZoneEntity
            {
                Connection = dnsConnection,
                ProviderZoneId = "zone-1",
                Name = "example.com",
            };
            db.Connections.Add(dnsConnection);
            db.DnsZones.Add(zone);
            db.DnsRecords.Add(new DnsRecordEntity
            {
                Zone = zone,
                Name = "manual.example.com",
                Type = "A",
                Value = "203.0.113.20",
                Ownership = DnsOwnershipNames.User,
                Enabled = true,
                DashboardEnabled = true,
                DashboardDisplayName = "Manual DNS",
            });
            db.FirewallHosts.Add(new FirewallHostEntity
            {
                ConnectionId = Guid.NewGuid(),
                Name = "edge-1",
                Domain = "edge.example.com",
                LastAppliedAtUtc = DateTimeOffset.UtcNow,
            });
            await db.SaveChangesAsync();
        }

        using var anonymousAdminClient = _factory.CreateClient(new WebApplicationFactoryClientOptions
        {
            BaseAddress = new Uri("http://localhost:8080"),
            HandleCookies = false,
            AllowAutoRedirect = false,
        });
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

        var adminAppsResponse = await anonymousAdminClient.GetAsync("/api/public/apps");
        var dashboardAppsResponse = await dashboardClient.GetAsync("/api/public/apps");
        Assert.Equal(HttpStatusCode.OK, adminAppsResponse.StatusCode);
        Assert.Equal(HttpStatusCode.OK, dashboardAppsResponse.StatusCode);
        var publicJson = await dashboardAppsResponse.Content.ReadAsStringAsync();
        var apps = JsonSerializer.Deserialize<PublicDashboardResponse>(publicJson, new JsonSerializerOptions { PropertyNameCaseInsensitive = true });
        Assert.NotNull(apps);
        Assert.Contains(apps!.Items, x => x.DisplayName == "Public App" && x.PublicUrl == "https://public.example.com");
        Assert.Contains(apps.Items, x => x.Source == "manual_dns" && x.DisplayName == "Manual DNS" && x.PublicUrl == "https://manual.example.com");
        Assert.Equal(2, apps.HostsOnline);
        Assert.Equal(2, apps.TotalHosts);
        Assert.Equal(1, apps.LinuxFirewallHostsAvailable);
        Assert.Equal(1, apps.TotalLinuxFirewallHosts);
        Assert.DoesNotContain("10.0.0.10", publicJson, StringComparison.Ordinal);
        Assert.DoesNotContain("targetHost", publicJson, StringComparison.Ordinal);
        Assert.DoesNotContain("targetPort", publicJson, StringComparison.Ordinal);
        Assert.DoesNotContain("firewallHostId", publicJson, StringComparison.Ordinal);
        Assert.DoesNotContain("pulseAgentId", publicJson, StringComparison.Ordinal);
        Assert.DoesNotContain("routes", publicJson, StringComparison.Ordinal);
        Assert.DoesNotContain("rules", publicJson, StringComparison.Ordinal);
        Assert.DoesNotContain("forwardAuthPolicy", publicJson, StringComparison.Ordinal);
        Assert.DoesNotContain("wafMode", publicJson, StringComparison.Ordinal);
        Assert.DoesNotContain("extraMiddlewares", publicJson, StringComparison.Ordinal);

        using var publicDoc = JsonDocument.Parse(publicJson);
        var firstItem = publicDoc.RootElement.GetProperty("items")[0];
        Assert.False(firstItem.TryGetProperty("targetHost", out _));
        Assert.False(firstItem.TryGetProperty("targetPort", out _));
        Assert.False(firstItem.TryGetProperty("firewallHostId", out _));
        Assert.False(firstItem.TryGetProperty("pulseAgentId", out _));
        Assert.False(firstItem.TryGetProperty("routes", out _));
        Assert.False(firstItem.TryGetProperty("rules", out _));
        Assert.False(firstItem.TryGetProperty("forwardAuthPolicy", out _));
        Assert.False(firstItem.TryGetProperty("wafMode", out _));
        Assert.False(firstItem.TryGetProperty("extraMiddlewares", out _));

        var statusItems = await statusClient.GetFromJsonAsync<IReadOnlyList<PublicStatusItemResponse>>("/api/public/status");
        Assert.NotNull(statusItems);
        Assert.Contains(statusItems!, x => x.Name == "Public App" && x.Status == "Up" && x.LastLatencyMs == 42);
        Assert.DoesNotContain(statusItems!, x => x.Name == "Private App");

        var adminFromDashboard = await dashboardClient.GetAsync("/api/resources");
        var adminFromStatus = await statusClient.GetAsync("/api/resources");
        var crossPortStatus = await dashboardClient.GetAsync("/api/public/status");
        Assert.Equal(HttpStatusCode.NotFound, adminFromDashboard.StatusCode);
        Assert.Equal(HttpStatusCode.NotFound, adminFromStatus.StatusCode);
        Assert.Equal(HttpStatusCode.NotFound, crossPortStatus.StatusCode);
    }

    [Fact]
    public async Task Public_status_disabled_returns_not_found_on_admin_and_status_ports()
    {
        if (!_fixture.IsAvailable || _factory is null || _client is null)
        {
            return;
        }

        await using (var scope = _factory.Services.CreateAsyncScope())
        {
            var db = scope.ServiceProvider.GetRequiredService<HashiDbContext>();
            var settings = await db.AppSettings.FindAsync(1);
            if (settings is null)
            {
                settings = new AppSettingsEntity();
                db.AppSettings.Add(settings);
            }

            settings.PublicStatusEnabled = false;
            db.MonitorEndpoints.Add(new MonitorEndpointEntity
            {
                Name = "Selected Public App",
                Url = "https://selected.example.com/",
                CheckType = "https",
                Enabled = true,
                PublicStatusEnabled = true,
                Status = "up",
            });
            await db.SaveChangesAsync();
        }

        using var statusClient = _factory.CreateClient(new WebApplicationFactoryClientOptions
        {
            BaseAddress = new Uri("http://localhost:8082"),
            HandleCookies = false,
            AllowAutoRedirect = false,
        });

        var adminStatus = await _client.GetAsync("/api/public/status");
        var adminSummary = await _client.GetAsync("/api/public/status/summary");
        var publicStatus = await statusClient.GetAsync("/api/public/status");
        var publicSummary = await statusClient.GetAsync("/api/public/status/summary");

        Assert.Equal(HttpStatusCode.NotFound, adminStatus.StatusCode);
        Assert.Equal(HttpStatusCode.NotFound, adminSummary.StatusCode);
        Assert.Equal(HttpStatusCode.NotFound, publicStatus.StatusCode);
        Assert.Equal(HttpStatusCode.NotFound, publicSummary.StatusCode);
    }

    [Fact]
    public async Task Public_dashboard_disabled_returns_not_found_on_admin_and_dashboard_ports()
    {
        if (!_fixture.IsAvailable || _factory is null)
        {
            return;
        }

        await using (var scope = _factory.Services.CreateAsyncScope())
        {
            var db = scope.ServiceProvider.GetRequiredService<HashiDbContext>();
            var settings = await db.AppSettings.FindAsync(1);
            if (settings is null)
            {
                settings = new AppSettingsEntity();
                db.AppSettings.Add(settings);
            }

            settings.PublicDashboardEnabled = false;
            db.Resources.Add(new ResourceEntity
            {
                Name = "Selected Public App",
                Slug = "selected-public-app",
                Kind = "https",
                Domain = "selected.example.com",
                TargetHost = "10.0.0.10",
                TargetPort = 8080,
                DashboardEnabled = true,
            });
            await db.SaveChangesAsync();
        }

        using var anonymousAdminClient = _factory.CreateClient(new WebApplicationFactoryClientOptions
        {
            BaseAddress = new Uri("http://localhost:8080"),
            HandleCookies = false,
            AllowAutoRedirect = false,
        });
        using var dashboardClient = _factory.CreateClient(new WebApplicationFactoryClientOptions
        {
            BaseAddress = new Uri("http://localhost:8081"),
            HandleCookies = false,
            AllowAutoRedirect = false,
        });

        var adminApps = await anonymousAdminClient.GetAsync("/api/public/apps");
        var publicApps = await dashboardClient.GetAsync("/api/public/apps");

        Assert.Equal(HttpStatusCode.NotFound, adminApps.StatusCode);
        Assert.Equal(HttpStatusCode.NotFound, publicApps.StatusCode);
    }

    private sealed record CsrfToken(string? Token);

    private static void AssertSetCookieFlag(HttpResponseMessage response, string cookieName, string flag)
    {
        var cookie = GetSetCookie(response, cookieName);
        Assert.Contains(
            cookie.Split(';', StringSplitOptions.TrimEntries | StringSplitOptions.RemoveEmptyEntries),
            value => string.Equals(value, flag, StringComparison.OrdinalIgnoreCase));
    }

    private static string GetSetCookie(HttpResponseMessage response, string cookieName)
    {
        Assert.True(response.Headers.TryGetValues("Set-Cookie", out var values));
        var cookie = values.FirstOrDefault(value =>
            value.StartsWith($"{cookieName}=", StringComparison.OrdinalIgnoreCase));
        Assert.NotNull(cookie);
        return cookie;
    }
}
