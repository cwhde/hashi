using System.Net;
using System.Security.Cryptography;
using System.Text;
using Hashi.Core.Auth;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Notifications;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Platform;
using Microsoft.EntityFrameworkCore;
using Xunit;

namespace Hashi.UnitTests;

public sealed class NotificationRoutingServiceTests
{
    [Fact]
    public async Task RouteMonitorTransition_requires_all_match_json_properties_to_match()
    {
        await using var db = CreateDb();
        var dispatcher = CreateDispatcher(db, out _);
        var provider = await AddDiscordProviderAsync(db, dispatcher);
        db.NotificationRoutes.Add(new NotificationRouteEntity
        {
            ProviderId = provider.Id,
            Name = "specific app",
            EventKind = "monitor",
            Severity = "critical",
            MatchJson = """{"name":"App","resourceId":"00000000-0000-0000-0000-000000000001"}""",
        });
        await db.SaveChangesAsync();
        var routing = new NotificationRoutingService(db, dispatcher);

        await routing.RouteMonitorTransitionAsync(new MonitorEndpointEntity
        {
            Name = "App",
            Url = "https://app.example.com",
            ResourceId = Guid.Parse("00000000-0000-0000-0000-000000000002"),
        }, "up", "down");

        Assert.Empty(await db.NotificationDeliveries.ToListAsync());
    }

    [Fact]
    public async Task RouteMonitorTransition_ignores_invalid_match_json_without_throwing()
    {
        await using var db = CreateDb();
        var dispatcher = CreateDispatcher(db, out _);
        var provider = await AddDiscordProviderAsync(db, dispatcher);
        db.NotificationRoutes.Add(new NotificationRouteEntity
        {
            ProviderId = provider.Id,
            Name = "broken route",
            EventKind = "monitor",
            Severity = "critical",
            MatchJson = "{",
        });
        await db.SaveChangesAsync();
        var routing = new NotificationRoutingService(db, dispatcher);

        await routing.RouteMonitorTransitionAsync(new MonitorEndpointEntity
        {
            Name = "App",
            Url = "https://app.example.com",
        }, "up", "down");

        Assert.Empty(await db.NotificationDeliveries.ToListAsync());
    }

    [Fact]
    public async Task RouteMonitorTransition_skips_disabled_providers()
    {
        await using var db = CreateDb();
        var dispatcher = CreateDispatcher(db, out _);
        var provider = await AddDiscordProviderAsync(db, dispatcher, enabled: false);
        db.NotificationRoutes.Add(new NotificationRouteEntity
        {
            ProviderId = provider.Id,
            Name = "disabled provider route",
            EventKind = "monitor",
            Severity = "critical",
            MatchJson = "{}",
        });
        await db.SaveChangesAsync();
        var routing = new NotificationRoutingService(db, dispatcher);

        await routing.RouteMonitorTransitionAsync(new MonitorEndpointEntity
        {
            Name = "App",
            Url = "https://app.example.com",
        }, "up", "down");

        Assert.Empty(await db.NotificationDeliveries.ToListAsync());
    }

    [Fact]
    public async Task RouteMonitorTransition_applies_cooldown_per_route_and_subject()
    {
        await using var db = CreateDb();
        var dispatcher = CreateDispatcher(db, out _);
        var provider = await AddDiscordProviderAsync(db, dispatcher);
        var route = new NotificationRouteEntity
        {
            ProviderId = provider.Id,
            Name = "cooldown route",
            EventKind = "monitor",
            Severity = "critical",
            MatchJson = "{}",
            CooldownMinutes = 60,
        };
        db.NotificationRoutes.Add(route);
        db.NotificationDeliveries.Add(new NotificationDeliveryEntity
        {
            RouteId = route.Id,
            ProviderId = provider.Id,
            EventKind = "monitor",
            Subject = "Monitor down: Other",
            Status = NotificationDeliveryStatusNames.Sent,
            CreatedAtUtc = DateTimeOffset.UtcNow.AddMinutes(-5),
            SentAtUtc = DateTimeOffset.UtcNow.AddMinutes(-5),
        });
        await db.SaveChangesAsync();
        var routing = new NotificationRoutingService(db, dispatcher);

        await routing.RouteMonitorTransitionAsync(new MonitorEndpointEntity
        {
            Name = "App",
            Url = "https://app.example.com",
        }, "up", "down");
        await routing.RouteMonitorTransitionAsync(new MonitorEndpointEntity
        {
            Name = "App",
            Url = "https://app.example.com",
        }, "up", "down");

        var appDeliveries = await db.NotificationDeliveries
            .Where(x => x.RouteId == route.Id && x.Subject == "Monitor down: App")
            .ToListAsync();
        Assert.Single(appDeliveries);
        Assert.Equal(NotificationDeliveryStatusNames.Sent, appDeliveries[0].Status);
    }

    [Fact]
    public async Task CreateRouteAsync_validates_route_inputs()
    {
        await using var db = CreateDb();
        var dispatcher = CreateDispatcher(db, out _);
        var provider = await AddDiscordProviderAsync(db, dispatcher);

        var invalid = await Assert.ThrowsAsync<InvalidOperationException>(() => dispatcher.CreateRouteAsync(
            new(
                provider.Id,
                "bad route",
                "monitor",
                "critical",
                "[1]",
                Enabled: true,
                CooldownMinutes: 0,
                SendRecovery: true)));
        Assert.Equal("Notification route match JSON must be a JSON object.", invalid.Message);

        var missingProvider = await Assert.ThrowsAsync<InvalidOperationException>(() => dispatcher.CreateRouteAsync(
            new(
                Guid.NewGuid(),
                "bad route",
                "monitor",
                "critical",
                "{}",
                Enabled: true,
                CooldownMinutes: 0,
                SendRecovery: true)));
        Assert.Equal("Notification provider not found.", missingProvider.Message);
    }

    private static HashiDbContext CreateDb()
    {
        var options = new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        return new HashiDbContext(options);
    }

    private static NotificationDispatcher CreateDispatcher(HashiDbContext db, out SecretRecordService secrets)
    {
        secrets = CreateSecrets(db);
        return new NotificationDispatcher(db, new FakeHttpClientFactory(), secrets);
    }

    private static async Task<NotificationProviderEntity> AddDiscordProviderAsync(
        HashiDbContext db,
        NotificationDispatcher dispatcher,
        bool enabled = true)
    {
        var response = await dispatcher.CreateProviderAsync(new(
            "Alerts",
            "discord",
            """{"webhookUrl":"https://discord.example/webhook"}""",
            enabled));
        return await db.NotificationProviders.SingleAsync(x => x.Id == response.Id);
    }

    private static SecretRecordService CreateSecrets(HashiDbContext db)
    {
        var vault = new VaultSessionState();
        vault.Unlock(RandomNumberGenerator.GetBytes(32));
        return new SecretRecordService(db, vault, new ServiceSyncVaultState());
    }

    private sealed class FakeHttpClientFactory : IHttpClientFactory
    {
        public HttpClient CreateClient(string name)
            => new(new OkHandler());
    }

    private sealed class OkHandler : HttpMessageHandler
    {
        protected override Task<HttpResponseMessage> SendAsync(HttpRequestMessage request, CancellationToken cancellationToken)
            => Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK)
            {
                Content = new StringContent("{}", Encoding.UTF8, "application/json"),
            });
    }
}
