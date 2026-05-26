using System.Net;
using System.Text;
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
        var dispatcher = new NotificationDispatcher(db, new FakeHttpClientFactory(client));

        var result = await dispatcher.DiscoverTelegramChatAsync("telegram-token");

        Assert.True(result.Found);
        Assert.Equal("-1001234567890", result.ChatId);
        Assert.Equal("Hashi Alerts", result.ChatTitle);
        Assert.Null(result.Error);
    }

    private static HashiDbContext CreateDb()
    {
        var options = new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        return new HashiDbContext(options);
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
