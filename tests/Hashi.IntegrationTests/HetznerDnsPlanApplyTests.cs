using System.Net.Http.Headers;
using System.Net;
using System.Net.Http.Json;
using System.Text.Json;
using Hashi.Contracts.Api;
using Hashi.Core.Dns;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Dns;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Providers.Dns;
using Hashi.Infrastructure.Services;
using Hashi.IntegrationTests.Fakes;
using Microsoft.AspNetCore.Hosting;
using Microsoft.AspNetCore.Mvc.Testing;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Logging;
using Microsoft.Extensions.Logging.Abstractions;
using Xunit;

namespace Hashi.IntegrationTests;

[Collection(PostgresIntegrationCollection.Name)]
public sealed class HetznerDnsPlanApplyTests : IAsyncLifetime
{
    private readonly PostgresIntegrationFixture _fixture;
    private readonly HetznerDnsFakeHandler _fake = new();
    private ServiceProvider? _services;
    private string? _connectionString;

    public HetznerDnsPlanApplyTests(PostgresIntegrationFixture fixture)
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
        var fakeClient = new HttpClient(_fake) { BaseAddress = new Uri("https://dns.fake/api/v1/") };
        fakeClient.DefaultRequestHeaders.Accept.Add(new MediaTypeWithQualityHeaderValue("application/json"));

        _services = new ServiceCollection()
            .AddDbContext<HashiDbContext>(options =>
                options.UseNpgsql(_connectionString, npgsql =>
                    npgsql.MigrationsAssembly(typeof(HashiDbContext).Assembly.FullName)))
            .AddSingleton<VaultSessionState>()
            .AddSingleton<ServiceSyncVaultState>()
            .AddSingleton<IHttpClientFactory>(_ => new SingleHttpClientFactory(fakeClient))
            .AddSingleton<IDnsProviderFactory, DnsProviderFactory>()
            .AddSingleton<ILogger<HetznerDnsProvider>>(_ => NullLogger<HetznerDnsProvider>.Instance)
            .AddScoped<SecretRecordService>()
            .AddScoped<AuditService>()
            .AddScoped<DnsConnectionService>()
            .BuildServiceProvider();

        using var scope = _services.CreateScope();
        var vault = scope.ServiceProvider.GetRequiredService<VaultSessionState>();
        vault.Unlock(new byte[32]);
    }

    public async Task DisposeAsync()
    {
        if (_services is not null)
        {
            await _services.DisposeAsync();
        }
    }

    [Fact]
    public async Task Plan_skips_ns_soa_deletes_and_apply_creates_managed_record()
    {
        if (!_fixture.IsAvailable || _services is null)
        {
            return;
        }

        using var scope = _services.CreateScope();
        var dns = scope.ServiceProvider.GetRequiredService<DnsConnectionService>();
        var db = scope.ServiceProvider.GetRequiredService<HashiDbContext>();

        var connection = await dns.CreateHetznerConnectionAsync("Test DNS", "fake-token", "example.com", 3600);
        var zone = await db.DnsZones.SingleAsync(x => x.ConnectionId == connection.Id);
        db.DnsRecords.Add(new DnsRecordEntity
        {
            ZoneId = zone.Id,
            ProviderRecordId = "managed-app",
            Name = "newapp",
            Type = "A",
            Value = "203.0.113.50",
            Ownership = DnsOwnershipNames.Managed,
        });
        await db.SaveChangesAsync();

        var plan = await dns.PlanSyncAsync(connection.Id);
        Assert.DoesNotContain(plan.Changes, x => x.Type is DnsRecordType.Ns or DnsRecordType.Soa && x.Kind == DnsChangeKind.Delete);
        Assert.Contains(plan.Changes, x => x.Kind == DnsChangeKind.Create && x.Name == "newapp");

        await dns.ApplyPlanAsync(plan, confirmDestructive: true);
        var providerRecords = await dns.ListProviderRecordsAsync(connection.Id);
        Assert.Contains(providerRecords, x => x.Name == "newapp" && x.Value == "203.0.113.50");
    }

    [Fact]
    public async Task Import_preview_includes_protected_records_as_not_selected()
    {
        if (!_fixture.IsAvailable || _services is null)
        {
            return;
        }

        using var scope = _services.CreateScope();
        var dns = scope.ServiceProvider.GetRequiredService<DnsConnectionService>();

        var connection = await dns.CreateHetznerConnectionAsync("Test DNS", "fake-token", "example.com", 3600);
        var decisions = await dns.BuildImportPreviewAsync(connection.Id);

        Assert.Contains(decisions, x => x.Type == "NS" && !x.SelectedForImport);
    }

    [Fact]
    public async Task Dns_sync_plan_then_apply_endpoint_accepts_returned_plan_id()
    {
        if (!_fixture.IsAvailable || _connectionString is null)
        {
            return;
        }

        await using var factory = CreateFactory();
        using var setupScope = factory.Services.CreateScope();
        setupScope.ServiceProvider.GetRequiredService<VaultSessionState>().Unlock(new byte[32]);
        await IntegrationTestAuth.EnsureBootstrapCredentialsAsync(factory.Services);

        using var client = factory.CreateClient(new WebApplicationFactoryClientOptions
        {
            HandleCookies = true,
            AllowAutoRedirect = false,
        });
        await IntegrationTestAuth.AuthenticateAsBootstrapAsync(client);
        IntegrationTestAuth.MarkRecentReauthentication(factory.Services);

        var createConnection = await SendPostWithCsrfAsync(
            client,
            "/api/dns/connections/hetzner",
            new CreateHetznerDnsConnectionRequest("Endpoint DNS", "fake-token", "example.com", 3600));
        Assert.Equal(HttpStatusCode.OK, createConnection.StatusCode);
        var connection = await createConnection.Content.ReadFromJsonAsync<ConnectionSummaryResponse>();
        Assert.NotNull(connection);

        using (var scope = factory.Services.CreateScope())
        {
            var db = scope.ServiceProvider.GetRequiredService<HashiDbContext>();
            var zone = await db.DnsZones.SingleAsync(x => x.ConnectionId == connection!.Id);
            db.DnsRecords.Add(new DnsRecordEntity
            {
                ZoneId = zone.Id,
                Name = "endpoint-new",
                Type = "A",
                Value = "203.0.113.60",
                Ownership = DnsOwnershipNames.Managed,
            });
            await db.SaveChangesAsync();
        }

        var planResponse = await SendPostWithCsrfAsync(client, $"/api/dns/connections/{connection!.Id}/sync/plan");
        Assert.Equal(HttpStatusCode.OK, planResponse.StatusCode);
        var plan = await planResponse.Content.ReadFromJsonAsync<DnsSyncPlanResponse>();
        Assert.NotNull(plan);
        Assert.NotEqual(Guid.Empty, plan!.PlanId);
        Assert.Equal(connection!.Id, plan.ConnectionId);
        Assert.Contains(plan!.Changes, x => x.Kind == nameof(DnsChangeKind.Create) && x.Name == "endpoint-new");

        var applyResponse = await SendPostWithCsrfAsync(
            client,
            $"/api/dns/connections/{connection.Id}/sync/apply",
            new DnsSyncApplyRequest(plan.PlanId, connection.Id, ConfirmDestructive: true));

        Assert.Equal(HttpStatusCode.OK, applyResponse.StatusCode);
        using var applyJson = await JsonDocument.ParseAsync(await applyResponse.Content.ReadAsStreamAsync());
        Assert.True(applyJson.RootElement.GetProperty("applied").GetBoolean());
        Assert.NotEqual(Guid.Empty, applyJson.RootElement.GetProperty("syncRunId").GetGuid());
        using var verifyScope = factory.Services.CreateScope();
        var verifyDb = verifyScope.ServiceProvider.GetRequiredService<HashiDbContext>();
        var record = await verifyDb.DnsRecords.SingleAsync(x => x.Name == "endpoint-new");
        Assert.False(string.IsNullOrWhiteSpace(record.ProviderRecordId));
        Assert.True(await verifyDb.SyncRuns.AnyAsync(x => x.Subsystem == "dns" && x.Status == SyncRunStatusNames.Succeeded));
        Assert.True(await verifyDb.SyncDiffs.AnyAsync(x => x.ResourceKey == "endpoint-new/A" && x.ChangeKind == nameof(DnsChangeKind.Create)));
    }

    [Fact]
    public async Task Plan_reports_unowned_matching_record_as_noop_and_apply_does_not_update_provider()
    {
        if (!_fixture.IsAvailable || _services is null)
        {
            return;
        }

        using var scope = _services.CreateScope();
        var dns = scope.ServiceProvider.GetRequiredService<DnsConnectionService>();
        var db = scope.ServiceProvider.GetRequiredService<HashiDbContext>();

        var connection = await dns.CreateHetznerConnectionAsync("Test DNS", "fake-token", "example.com", 3600);
        var zone = await db.DnsZones.SingleAsync(x => x.ConnectionId == connection.Id);
        db.DnsRecords.Add(new DnsRecordEntity
        {
            ZoneId = zone.Id,
            Name = "app",
            Type = "A",
            Value = "203.0.113.70",
            Ownership = DnsOwnershipNames.Managed,
        });
        await db.SaveChangesAsync();

        var plan = await dns.PlanSyncAsync(connection.Id);
        Assert.Contains(plan.Changes, x =>
            x.Name == "app"
            && x.Kind == DnsChangeKind.NoOp
            && x.RiskReason.Contains("not owned", StringComparison.OrdinalIgnoreCase));
        Assert.DoesNotContain(plan.Changes, x => x.Name == "app" && x.Kind == DnsChangeKind.Update);

        await dns.ApplyPlanAsync(plan, confirmDestructive: true);
        var providerRecords = await dns.ListProviderRecordsAsync(connection.Id);
        Assert.Contains(providerRecords, x => x.Name == "app" && x.Value == "1.2.3.4");
        Assert.DoesNotContain(providerRecords, x => x.Name == "app" && x.Value == "203.0.113.70");
    }

    private WebApplicationFactory<Program> CreateFactory()
    {
        return IntegrationTestApp.CreateFactory(_connectionString!)
            .WithWebHostBuilder(builder =>
            {
                builder.ConfigureServices(services =>
                {
                    services.AddHttpClient("hetzner-dns")
                        .ConfigurePrimaryHttpMessageHandler(() => _fake);
                });
            });
    }

    private static async Task<HttpResponseMessage> SendPostWithCsrfAsync(
        HttpClient client,
        string path,
        object? body = null)
    {
        var csrf = await client.GetFromJsonAsync<CsrfToken>("/api/auth/csrf");
        var request = new HttpRequestMessage(HttpMethod.Post, path);
        if (body is not null)
        {
            request.Content = JsonContent.Create(body);
        }

        if (!string.IsNullOrEmpty(csrf?.Token))
        {
            request.Headers.Add("X-CSRF-TOKEN", csrf.Token);
        }

        return await client.SendAsync(request);
    }

    private sealed class SingleHttpClientFactory(HttpClient client) : IHttpClientFactory
    {
        public HttpClient CreateClient(string name) => client;
    }

    private sealed record CsrfToken(string? Token);
}
