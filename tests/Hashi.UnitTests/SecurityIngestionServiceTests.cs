using Hashi.Contracts.Api;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Notifications;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Platform;
using Hashi.Infrastructure.Services;
using Hashi.UnitTests.Fakes;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Xunit;

namespace Hashi.UnitTests;

public sealed class SecurityIngestionServiceTests
{
    [Fact]
    public async Task GetDashboardAsync_aggregates_counts_and_rankings()
    {
        await using var db = CreateDb();
        db.AccessLogEvents.AddRange(
            Event("1.1.1.1", "allowed", "US", "AS13335"),
            Event("1.1.1.1", "blocked", "US", "AS13335"),
            Event("2.2.2.2", "blocked", "DE", "AS24940"),
            Event("3.3.3.3", "challenged", "US", "AS13335"));
        await db.SaveChangesAsync();

        var service = CreateService(db);
        var dashboard = await service.GetDashboardAsync(24);

        Assert.Equal(1, dashboard.Allowed);
        Assert.Equal(2, dashboard.Blocked);
        Assert.Equal(1, dashboard.Challenged);
        Assert.Equal(24, dashboard.Hours);
        Assert.Contains("1.1.1.1", dashboard.TopBlockedIps);
        Assert.Equal("US", dashboard.TopCountries[0].Label);
        Assert.Equal(3, dashboard.TopCountries[0].Count);
        Assert.Equal("AS13335", dashboard.TopAsns[0].Label);
        Assert.Equal(3, dashboard.TopAsns[0].Count);
    }

    [Fact]
    public async Task IngestForwardAuthDecisionAsync_records_challenged_event()
    {
        await using var db = CreateDb();
        var service = CreateService(db);

        await service.IngestForwardAuthDecisionAsync(new ForwardAuthDecisionIngestRequest(
            "203.0.113.10",
            "app.example.com",
            "/",
            "challenge",
            "US",
            "AS13335"));

        var stored = await db.AccessLogEvents.SingleAsync();
        Assert.Equal("challenged", stored.Decision);
        Assert.Equal(401, stored.StatusCode);
    }

    [Fact]
    public async Task IngestAccessLogAsync_promotes_abuse_bucket_to_block()
    {
        await using var db = CreateDb();
        var service = CreateService(db);

        for (var i = 0; i < 12; i++)
        {
            await service.IngestAccessLogAsync(new AccessLogIngestRequest(
                "198.51.100.50",
                "app.example.com",
                "/",
                404,
                "US",
                "AS13335"));
        }

        var bucket = await db.AbuseBuckets.SingleAsync();
        Assert.Equal("block", bucket.State);
        Assert.Single(await db.BlocklistEntries.ToListAsync());
    }

    private static AccessLogEventEntity Event(string ip, string decision, string country, string asn)
        => new()
        {
            ClientIp = ip,
            Host = "app.example.com",
            Path = "/",
            StatusCode = decision == "blocked" ? 403 : 200,
            CountryCode = country,
            Asn = asn,
            Decision = decision,
            ReceivedAtUtc = DateTimeOffset.UtcNow,
        };

    private static HashiDbContext CreateDb()
    {
        var options = new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        return new HashiDbContext(options);
    }

    private static SecurityIngestionService CreateService(HashiDbContext db)
    {
        var audit = new AuditService(db);
        var secrets = new SecretRecordService(db, new VaultSessionState(), new ServiceSyncVaultState());
        var dispatcher = new NotificationDispatcher(db, new ServiceCollection().AddHttpClient().BuildServiceProvider().GetRequiredService<IHttpClientFactory>());
        var routing = new NotificationRoutingService(db, dispatcher);
        return new SecurityIngestionService(
            db,
            TestPlatformHelpers.CreateFirewallApply(db),
            audit,
            routing,
            Microsoft.Extensions.Logging.Abstractions.NullLogger<SecurityIngestionService>.Instance);
    }
}
