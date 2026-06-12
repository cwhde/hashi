using System.Net.Http.Headers;
using System.Net.WebSockets;
using System.Text;
using System.Text.Json;

namespace Hashi.Infrastructure.Notifications;

public sealed record DiscordDiscoveredChannel(string ChannelId, string? ChannelName, string UserId);

public interface IDiscordChannelDiscovery
{
    Task<DiscordDiscoveredChannel?> DiscoverAsync(string botToken, TimeSpan timeout, CancellationToken cancellationToken);
}

public sealed class DiscordChannelDiscovery(IHttpClientFactory httpClientFactory) : IDiscordChannelDiscovery
{
    private const int RequiredIntents = 512 | 4096;

    public async Task<DiscordDiscoveredChannel?> DiscoverAsync(
        string botToken,
        TimeSpan timeout,
        CancellationToken cancellationToken)
    {
        var client = httpClientFactory.CreateClient();
        using var gatewayRequest = new HttpRequestMessage(HttpMethod.Get, "https://discord.com/api/v10/gateway/bot");
        gatewayRequest.Headers.Authorization = new AuthenticationHeaderValue("Bot", botToken);
        using var gatewayResponse = await client.SendAsync(gatewayRequest, cancellationToken);
        gatewayResponse.EnsureSuccessStatusCode();
        using var gatewayDocument = JsonDocument.Parse(await gatewayResponse.Content.ReadAsStringAsync(cancellationToken));
        var gatewayUrl = gatewayDocument.RootElement.GetProperty("url").GetString()
            ?? throw new InvalidOperationException("Discord did not return a Gateway URL.");

        using var socket = new ClientWebSocket();
        using var timeoutSource = CancellationTokenSource.CreateLinkedTokenSource(cancellationToken);
        timeoutSource.CancelAfter(timeout);
        var token = timeoutSource.Token;
        await socket.ConnectAsync(new Uri($"{gatewayUrl.TrimEnd('/')}?v=10&encoding=json"), token);

        var hello = await ReceiveJsonAsync(socket, token);
        if (!hello.RootElement.TryGetProperty("op", out var helloOp) || helloOp.GetInt32() != 10)
        {
            throw new InvalidOperationException("Discord Gateway did not send a Hello event.");
        }

        await SendJsonAsync(socket, new
        {
            op = 2,
            d = new
            {
                token = botToken,
                intents = RequiredIntents,
                properties = new Dictionary<string, string>
                {
                    ["os"] = Environment.OSVersion.Platform.ToString(),
                    ["browser"] = "hashi",
                    ["device"] = "hashi",
                },
            },
        }, token);

        string? botUserId = null;
        while (!token.IsCancellationRequested)
        {
            using var message = await ReceiveJsonAsync(socket, token);
            var root = message.RootElement;
            if (!root.TryGetProperty("op", out var op) || op.GetInt32() != 0 ||
                !root.TryGetProperty("t", out var typeElement))
            {
                continue;
            }

            var eventType = typeElement.GetString();
            var data = root.GetProperty("d");
            if (eventType == "READY")
            {
                botUserId = data.GetProperty("user").GetProperty("id").GetString();
                continue;
            }

            if (eventType != "MESSAGE_CREATE" || botUserId is null ||
                !data.TryGetProperty("author", out var author) ||
                (author.TryGetProperty("bot", out var bot) && bot.GetBoolean()))
            {
                continue;
            }

            var isDirectMessage = !data.TryGetProperty("guild_id", out _);
            var mentionsBot = data.TryGetProperty("mentions", out var mentions) &&
                mentions.ValueKind == JsonValueKind.Array &&
                mentions.EnumerateArray().Any(x => x.TryGetProperty("id", out var id) && id.GetString() == botUserId);
            if (!isDirectMessage && !mentionsBot)
            {
                continue;
            }

            return new DiscordDiscoveredChannel(
                data.GetProperty("channel_id").GetString()!,
                data.TryGetProperty("channel_name", out var channelName) ? channelName.GetString() : null,
                author.GetProperty("id").GetString()!);
        }

        return null;
    }

    private static async Task<JsonDocument> ReceiveJsonAsync(ClientWebSocket socket, CancellationToken cancellationToken)
    {
        using var stream = new MemoryStream();
        var buffer = new byte[8192];
        WebSocketReceiveResult result;
        do
        {
            result = await socket.ReceiveAsync(buffer, cancellationToken);
            if (result.MessageType == WebSocketMessageType.Close)
            {
                throw new InvalidOperationException("Discord Gateway closed the pairing connection.");
            }
            stream.Write(buffer, 0, result.Count);
        } while (!result.EndOfMessage);

        stream.Position = 0;
        return await JsonDocument.ParseAsync(stream, cancellationToken: cancellationToken);
    }

    private static Task SendJsonAsync(ClientWebSocket socket, object payload, CancellationToken cancellationToken)
    {
        var bytes = Encoding.UTF8.GetBytes(JsonSerializer.Serialize(payload));
        return socket.SendAsync(bytes, WebSocketMessageType.Text, true, cancellationToken);
    }
}
