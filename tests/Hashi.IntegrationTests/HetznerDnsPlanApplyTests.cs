using System.Net.Http.Headers;
using Hashi.Core.Dns;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Dns;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Providers.Dns;
using Hashi.Infrastructure.Services;
using Hashi.IntegrationTests.Fakes;
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

        var connectionString = await _fixture.CreateDatabaseAsync();
        var fakeClient = new HttpClient(_fake) { BaseAddress = new Uri("https://dns.fake/api/v1/") };
        fakeClient.DefaultRequestHeaders.Accept.Add(new MediaTypeWithQualityHeaderValue("application/json"));

        _services = new ServiceCollection()
            .AddDbContext<HashiDbContext>(options =>
                options.UseNpgsql(connectionString, npgsql =>
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
        var db = scope.ServiceProvider.GetRequiredService<HashiDbContext>();
        db.AppSettings.Add(new AppSettingsEntity());
        db.SetupStates.Add(new SetupStateEntity());
        await db.SaveChangesAsync();

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

    private sealed class SingleHttpClientFactory(HttpClient client) : IHttpClientFactory
    {
        public HttpClient CreateClient(string name) => client;
    }
}
