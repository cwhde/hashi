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
        var now = DateTimeOffset.UtcNow;
        db.SecurityRequestBuckets.AddRange(
            Bucket("1.1.1.1", "app.example.com", "default", "US", "CA", "AS13335", statusClass: 2, "GET", "/", 1, 1, 0, now),
            Bucket("2.2.2.2", "app.example.com", "default", "DE", "BE", "AS24940", statusClass: 4, "GET", "/", 0, 1, 0, now),
            Bucket("3.3.3.3", "app.example.com", "default", "US", "CA", "AS13335", statusClass: 2, "GET", "/", 0, 0, 1, now));
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
    public async Task SecurityRequestBuckets_groups_events_by_minute_and_dimensions()
    {
        await using var db = CreateDb();
        var service = CreateService(db);

        for (var i = 0; i < 500; i++)
        {
            await service.IngestAccessLogAsync(new AccessLogIngestRequest(
                "203.0.113.25",
                "api.example.com",
                "/v1/users",
                429,
                "US",
                "AS13335",
                RegionCode: "CA",
                Method: "GET",
                PathPrefix: "/v1",
                TraefikInstance: "traefik-a",
                Resource: "resource-api"));
        }

        var buckets = await db.SecurityRequestBuckets.ToListAsync();
        Assert.Single(buckets);
        var bucket = buckets[0];
        Assert.Equal(500, bucket.TotalCount);
        Assert.Equal(4, bucket.AllowedCount);
        Assert.Equal(491, bucket.BlockedCount);
        Assert.Equal(5, bucket.ChallengedCount);
        Assert.Equal("203.0.113.25", bucket.ClientIp);
        Assert.Equal("resource-api", bucket.Resource);
        Assert.Equal("traefik-a", bucket.TraefikInstance);
        Assert.Equal("US", bucket.CountryCode);
        Assert.Equal("CA", bucket.RegionCode);
        Assert.Equal("AS13335", bucket.Asn);
        Assert.Equal(4, bucket.StatusClass);
        Assert.Equal("GET", bucket.Method);
        Assert.Equal("/v1", bucket.PathPrefix);
    }

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

    private static SecurityRequestBucketEntity Bucket(
        string ip,
        string resource,
        string traefikInstance,
        string country,
        string region,
        string asn,
        int statusClass,
        string method,
        string pathPrefix,
        long allowed,
        long blocked,
        long challenged,
        DateTimeOffset bucketStartUtc)
        => new()
        {
            BucketStartUtc = bucketStartUtc,
            ClientIp = ip,
            Resource = resource,
            TraefikInstance = traefikInstance,
            CountryCode = country,
            RegionCode = region,
            Asn = asn,
            StatusClass = statusClass,
            Method = method,
            PathPrefix = pathPrefix,
            TotalCount = allowed + blocked + challenged,
            AllowedCount = allowed,
            BlockedCount = blocked,
            ChallengedCount = challenged,
            UpdatedAtUtc = bucketStartUtc,
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
