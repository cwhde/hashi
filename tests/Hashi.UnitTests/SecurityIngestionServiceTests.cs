using Hashi.Contracts.Api;
using Hashi.Core.Auth;
using Hashi.Core.Connections;
using System.Text.Json;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Connections;
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
        var topBlockedIp = Assert.Single(dashboard.TopBlockedIps, x => x.Ip == "1.1.1.1");
        Assert.Equal(1, topBlockedIp.Count);
        Assert.Equal("US", topBlockedIp.CountryCode);
        Assert.Equal("AS13335", topBlockedIp.Asn);
        Assert.Equal("US", dashboard.TopCountries[0].Label);
        Assert.Equal(3, dashboard.TopCountries[0].Count);
        Assert.Equal("AS13335", dashboard.TopAsns[0].Label);
        Assert.Equal(3, dashboard.TopAsns[0].Count);
    }

    [Fact]
    public async Task SecurityRequestBuckets_groups_events_by_minute_and_dimensions()
    {
        await using var db = CreateDb();
        var service = CreateService(db, new FixedTimeProvider(new DateTimeOffset(2026, 5, 27, 6, 0, 30, TimeSpan.Zero)));

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

    [Fact]
    public async Task GetDashboardAsync_returns_waf_counts_distinct_from_access_logs()
    {
        await using var db = CreateDb();
        db.AccessLogEvents.AddRange(
            Event("1.1.1.1", "blocked", "US", "AS13335"),
            Event("2.2.2.2", "blocked", "US", "AS13335"));
        db.SecurityEvents.Add(new SecurityEventEntity
        {
            Category = "waf",
            Action = "blocked",
            ClientIp = "3.3.3.3",
            Host = "app.example.com",
            Path = "/admin",
        });
        await db.SaveChangesAsync();

        var service = CreateService(db);
        var dashboard = await service.GetDashboardAsync(24);

        Assert.Equal(2, dashboard.Blocked);
        Assert.Equal(1, dashboard.WafDetections);
        Assert.Equal(1, dashboard.WafBlocks);
    }

    [Fact]
    public async Task IngestForwardAuthDecisionAsync_records_challenged_event()
    {
        await using var db = CreateDb();
        var service = CreateService(db);

        await service.IngestForwardAuthDecisionAsync(new ForwardAuthDecisionIngestRequest(
            "203.0.113.10",
            "app.example.com",
            "/v1/orders/42",
            "challenge",
            "US",
            "AS13335",
            RegionCode: "CA",
            Method: "POST",
            PathPrefix: "/v1"));

        var stored = await db.AccessLogEvents.SingleAsync();
        Assert.Equal("challenged", stored.Decision);
        Assert.Equal(401, stored.StatusCode);
        var bucket = await db.SecurityRequestBuckets.SingleAsync();
        Assert.Equal("POST", bucket.Method);
        Assert.Equal("/v1", bucket.PathPrefix);
        Assert.Equal("CA", bucket.RegionCode);
        Assert.Equal(1, bucket.ChallengedCount);
    }

    [Fact]
    public async Task IngestWafEventAsync_records_security_event()
    {
        await using var db = CreateDb();
        var service = CreateService(db);

        await service.IngestWafEventAsync(new WafEventIngestRequest(
            "203.0.113.10",
            "app.example.com",
            "/admin",
            "deny"));

        var stored = await db.SecurityEvents.SingleAsync();
        Assert.Equal("waf", stored.Category);
        Assert.Equal("blocked", stored.Action);
        Assert.Equal("203.0.113.10", stored.ClientIp);
        Assert.Equal("app.example.com", stored.Host);
        Assert.Equal("/admin", stored.Path);
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
        var block = Assert.Single(await db.BlocklistEntries.ToListAsync());
        Assert.Equal(BlocklistScopeNames.Global, block.Scope);
        Assert.Equal(BlocklistTypeNames.Ip, block.Type);
        Assert.Equal("198.51.100.50", block.Value);
        Assert.Equal("abuse_score_threshold", block.Reason);
        Assert.Equal(BlocklistSourceNames.Automatic, block.Source);
        Assert.Equal("hashi", block.CreatedBy);
    }

    [Fact]
    public async Task SyncBlocklistToAllFirewallsAsync_records_per_host_applied_state_for_active_ip_blocks()
    {
        await using var db = CreateDb();
        var vault = new VaultSessionState();
        vault.Unlock(new byte[32]);
        var secretService = new SecretRecordService(db, vault, new ServiceSyncVaultState());
        var credential = await secretService.StoreAsync(
            SecretPurpose.SshCredential,
            "fw1 ssh",
            ConnectionSshCredentialResolver.SerializeCredentialPayload("password", "secret", null, null));
        var connection = new ConnectionEntity
        {
            Name = "fw1",
            Type = ConnectionTypeNames.FirewallHost,
            SecretId = credential.Id,
            SettingsJson = JsonSerializer.Serialize(new
            {
                Host = "203.0.113.5",
                Port = 22,
                Username = "root",
            }),
        };
        db.Connections.Add(connection);
        var host = new FirewallHostEntity
        {
            ConnectionId = connection.Id,
            Name = "fw1",
            Domain = "example.com",
            ManagedSubnetsJson = "[]",
            LinkedTraefikHost = "traefik.local",
            InternalTraefikIp = "10.0.0.2",
        };
        var activeIpBlock = new BlocklistEntryEntity
        {
            Type = BlocklistTypeNames.Ip,
            Value = "198.51.100.50",
            ClientIp = "198.51.100.50",
            Reason = "abuse",
        };
        db.FirewallHosts.Add(host);
        db.BlocklistEntries.AddRange(
            activeIpBlock,
            new BlocklistEntryEntity { Type = BlocklistTypeNames.Asn, Value = "AS13335", Reason = "asn" },
            new BlocklistEntryEntity
            {
                Type = BlocklistTypeNames.Ip,
                Value = "198.51.100.51",
                ClientIp = "198.51.100.51",
                Reason = "expired",
                ExpiresAtUtc = DateTimeOffset.UtcNow.AddMinutes(-1),
            });
        await db.SaveChangesAsync();
        var ssh = new FakeSshRemoteExecutor();
        ssh.CommandResults.Enqueue(new RemoteCommandResult(true, "hashi-firewall-preflight-ok", null));
        ssh.CommandResults.Enqueue(new RemoteCommandResult(true, "no", null));
        ssh.CommandResults.Enqueue(new RemoteCommandResult(true, string.Empty, null));
        ssh.CommandResults.Enqueue(new RemoteCommandResult(true, string.Empty, null));
        ssh.CommandResults.Enqueue(new RemoteCommandResult(true, string.Empty, null));
        var service = CreateService(db, ssh: ssh, vault: vault);

        var result = await service.SyncBlocklistToAllFirewallsAsync();

        Assert.True(result.Synced);
        Assert.Equal(1, result.PendingEntries);
        var applied = await db.BlocklistAppliedHosts.SingleAsync();
        Assert.Equal(activeIpBlock.Id, applied.BlocklistEntryId);
        Assert.Equal(host.Id, applied.FirewallHostId);
        Assert.Equal(BlocklistApplyStatusNames.Applied, applied.Status);
        Assert.NotNull(applied.AppliedAtUtc);
        Assert.True((await db.BlocklistEntries.SingleAsync(x => x.Id == activeIpBlock.Id)).SyncedToFirewall);
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

    private static SecurityIngestionService CreateService(
        HashiDbContext db,
        TimeProvider? timeProvider = null,
        FakeSshRemoteExecutor? ssh = null,
        VaultSessionState? vault = null)
    {
        var audit = new AuditService(db);
        vault ??= new VaultSessionState();
        var secrets = new SecretRecordService(db, vault, new ServiceSyncVaultState());
        var dispatcher = new NotificationDispatcher(
            db,
            new ServiceCollection().AddHttpClient().BuildServiceProvider().GetRequiredService<IHttpClientFactory>(),
            secrets);
        var routing = new NotificationRoutingService(db, dispatcher);
        return new SecurityIngestionService(
            db,
            TestPlatformHelpers.CreateFirewallApply(db, ssh, vault),
            audit,
            routing,
            Microsoft.Extensions.Logging.Abstractions.NullLogger<SecurityIngestionService>.Instance,
            timeProvider);
    }

    private sealed class FixedTimeProvider(DateTimeOffset utcNow) : TimeProvider
    {
        public override DateTimeOffset GetUtcNow() => utcNow;
    }
}
