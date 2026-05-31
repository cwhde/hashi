using System.Net;
using System.Security.Cryptography;
using System.Text;
using Hashi.Contracts.Api;
using Hashi.Core.Auth;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Notifications;
using Hashi.Infrastructure.Persistence;
using Microsoft.EntityFrameworkCore;
using Xunit;

namespace Hashi.UnitTests;

public sealed class NotificationDispatcherTests
{
    [Fact]
    public async Task TelegramChatDiscovery_returns_chat_id_from_getUpdates()
    {
        await using var db = CreateDb();
        var client = new HttpClient(new TelegramGetUpdatesFakeHandler());
        var dispatcher = CreateDispatcher(db, client);

        var result = await dispatcher.DiscoverTelegramChatAsync("telegram-token");

        Assert.True(result.Found);
        Assert.Equal("-1001234567890", result.ChatId);
        Assert.Equal("Hashi Alerts", result.ChatTitle);
        Assert.Null(result.Error);
    }

    [Theory]
    [InlineData("smtp", """{"host":"smtp.example.com","port":587,"username":"mailer","password":"smtp-secret","from":"hashi@example.com","to":"admin@example.com","useTls":true}""", "smtp-secret", "password", "passwordSecretId")]
    [InlineData("telegram", """{"botToken":"telegram-secret","chatId":"-100123"}""", "telegram-secret", "botToken", "botTokenSecretId")]
    [InlineData("discord", """{"webhookUrl":"https://discord.example/webhook-secret"}""", "webhook-secret", "webhookUrl", "webhookSecretId")]
    public async Task CreateProviderAsync_moves_notification_secrets_to_secret_records(
        string type,
        string settingsJson,
        string plaintextSecret,
        string plaintextProperty,
        string secretIdProperty)
    {
        await using var db = CreateDb();
        var dispatcher = CreateDispatcher(db);

        var response = await dispatcher.CreateProviderAsync(new CreateNotificationProviderRequest(
            "Alerts",
            type,
            settingsJson,
            Enabled: true));

        var stored = await db.NotificationProviders.SingleAsync(x => x.Id == response.Id);
        Assert.DoesNotContain(plaintextSecret, stored.SettingsJson, StringComparison.Ordinal);
        Assert.DoesNotContain($"\"{plaintextProperty}\"", stored.SettingsJson, StringComparison.Ordinal);
        Assert.Contains(secretIdProperty, stored.SettingsJson, StringComparison.Ordinal);
        Assert.Single(await db.SecretRecords.ToListAsync());
    }

    [Fact]
    public async Task UpdateProviderAsync_preserves_existing_secret_when_no_new_secret_is_submitted()
    {
        await using var db = CreateDb();
        var dispatcher = CreateDispatcher(db);
        var created = await dispatcher.CreateProviderAsync(new CreateNotificationProviderRequest(
            "Alerts",
            "telegram",
            """{"botToken":"telegram-secret","chatId":"-100123"}""",
            Enabled: true));
        var before = await db.NotificationProviders.AsNoTracking().SingleAsync(x => x.Id == created.Id);

        await dispatcher.UpdateProviderAsync(
            created.Id,
            new UpdateNotificationProviderRequest(
                Name: null,
                Type: null,
                SettingsJson: """{"chatId":"-100456"}""",
                Enabled: null));

        var after = await db.NotificationProviders.AsNoTracking().SingleAsync(x => x.Id == created.Id);
        Assert.Contains("\"chatId\":\"-100456\"", after.SettingsJson, StringComparison.Ordinal);
        Assert.Contains(ExtractJsonString(before.SettingsJson, "botTokenSecretId"), after.SettingsJson, StringComparison.Ordinal);
        Assert.Single(await db.SecretRecords.ToListAsync());
    }

    [Fact]
    public async Task CreateProviderAsync_stores_notification_token_for_service_sync()
    {
        await using var db = CreateDb();
        var serviceSync = ReadyServiceSync();
        var secrets = CreateSecrets(db, serviceSync);
        var dispatcher = new NotificationDispatcher(db, new FakeHttpClientFactory(new HttpClient()), secrets);

        await dispatcher.CreateProviderAsync(new CreateNotificationProviderRequest(
            "Alerts",
            "telegram",
            """{"botToken":"telegram-secret","chatId":"-100123"}""",
            Enabled: true));

        var secret = await db.SecretRecords.SingleAsync();
        Assert.True(secret.IsServiceSyncEligible);
        Assert.NotNull(secret.ServiceWrappedDekBlob);
        Assert.Equal("telegram-secret", Encoding.UTF8.GetString((await secrets.DecryptForServiceSyncAsync(secret.Id))!));
    }

    private static string ExtractJsonString(string json, string property)
    {
        using var doc = System.Text.Json.JsonDocument.Parse(json);
        return doc.RootElement.GetProperty(property).GetString()!;
    }

    private static HashiDbContext CreateDb()
    {
        var options = new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        return new HashiDbContext(options);
    }

    private static NotificationDispatcher CreateDispatcher(HashiDbContext db, HttpClient? client = null)
        => new(db, new FakeHttpClientFactory(client ?? new HttpClient()), CreateSecrets(db, new ServiceSyncVaultState()));

    private static SecretRecordService CreateSecrets(HashiDbContext db, ServiceSyncVaultState serviceSync)
    {
        var vault = new VaultSessionState();
        vault.Unlock(RandomNumberGenerator.GetBytes(32));
        return new SecretRecordService(db, vault, serviceSync);
    }

    private static ServiceSyncVaultState ReadyServiceSync()
    {
        var serviceSync = new ServiceSyncVaultState();
        serviceSync.Initialize(RandomNumberGenerator.GetBytes(32));
        return serviceSync;
    }

    private sealed class FakeHttpClientFactory(HttpClient client) : IHttpClientFactory
    {
        public HttpClient CreateClient(string name) => client;
    }

    private sealed class TelegramGetUpdatesFakeHandler : HttpMessageHandler
    {
        protected override Task<HttpResponseMessage> SendAsync(HttpRequestMessage request, CancellationToken cancellationToken)
        {
            Assert.NotNull(request.RequestUri);
            Assert.Contains("/getUpdates", request.RequestUri!.AbsoluteUri);

            const string payload = """
            {
              "ok": true,
              "result": [
                {
                  "update_id": 1001,
                  "message": {
                    "chat": {
                      "id": -1001234567890,
                      "title": "Hashi Alerts"
                    }
                  }
                }
              ]
            }
            """;
            return Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK)
            {
                Content = new StringContent(payload, Encoding.UTF8, "application/json"),
            });
        }
    }
}
