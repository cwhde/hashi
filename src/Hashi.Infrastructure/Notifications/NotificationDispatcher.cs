using System.Net;
using System.Net.Http.Json;
using System.Net.Mail;
using System.Text;
using System.Text.Json;
using Hashi.Contracts.Api;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Infrastructure.Notifications;

public sealed class NotificationDispatcher(HashiDbContext db, IHttpClientFactory httpClientFactory)
{
    public async Task<IReadOnlyList<NotificationProviderResponse>> ListProvidersAsync(CancellationToken cancellationToken = default)
    {
        var providers = await db.NotificationProviders.AsNoTracking().OrderBy(x => x.Name).ToListAsync(cancellationToken);
        return providers.Select(x => new NotificationProviderResponse(x.Id, x.Name, x.Type, x.Enabled)).ToList();
    }

    public async Task<NotificationProviderResponse> CreateProviderAsync(
        CreateNotificationProviderRequest request,
        CancellationToken cancellationToken = default)
    {
        var entity = new NotificationProviderEntity
        {
            Name = request.Name,
            Type = request.Type,
            SettingsJson = request.SettingsJson,
            Enabled = request.Enabled,
        };
        db.NotificationProviders.Add(entity);
        await db.SaveChangesAsync(cancellationToken);
        return new NotificationProviderResponse(entity.Id, entity.Name, entity.Type, entity.Enabled);
    }

    public async Task<NotificationProviderResponse?> UpdateProviderAsync(
        Guid providerId,
        UpdateNotificationProviderRequest request,
        CancellationToken cancellationToken = default)
    {
        var entity = await db.NotificationProviders.SingleOrDefaultAsync(x => x.Id == providerId, cancellationToken);
        if (entity is null)
        {
            return null;
        }

        if (request.Name is not null)
        {
            entity.Name = request.Name;
        }

        if (request.Type is not null)
        {
            entity.Type = request.Type;
        }

        if (request.SettingsJson is not null)
        {
            entity.SettingsJson = request.SettingsJson;
        }

        if (request.Enabled is bool enabled)
        {
            entity.Enabled = enabled;
        }

        await db.SaveChangesAsync(cancellationToken);
        return new NotificationProviderResponse(entity.Id, entity.Name, entity.Type, entity.Enabled);
    }

    public async Task<bool> DeleteProviderAsync(Guid providerId, CancellationToken cancellationToken = default)
    {
        var entity = await db.NotificationProviders.SingleOrDefaultAsync(x => x.Id == providerId, cancellationToken);
        if (entity is null)
        {
            return false;
        }

        db.NotificationProviders.Remove(entity);
        await db.SaveChangesAsync(cancellationToken);
        return true;
    }

    public async Task<NotificationTestResponse> TestProviderAsync(
        Guid providerId,
        NotificationTestRequest request,
        CancellationToken cancellationToken = default)
    {
        var provider = await db.NotificationProviders.SingleOrDefaultAsync(x => x.Id == providerId, cancellationToken);
        if (provider is null)
        {
            return new NotificationTestResponse(false, "Provider not found.");
        }

        try
        {
            switch (provider.Type)
            {
                case "smtp":
                    await SendSmtpAsync(provider, request.Subject, request.Body, cancellationToken);
                    break;
                case "telegram":
                    await SendTelegramAsync(provider, request.Subject, request.Body, cancellationToken);
                    break;
                case "discord":
                    await SendDiscordAsync(provider, request.Subject, request.Body, cancellationToken);
                    break;
                default:
                    return new NotificationTestResponse(false, $"Unsupported provider type: {provider.Type}");
            }

            return new NotificationTestResponse(true, null);
        }
        catch (Exception ex)
        {
            return new NotificationTestResponse(false, ex.Message);
        }
    }

    public async Task<TelegramChatDiscoveryResponse> DiscoverTelegramChatAsync(
        string botToken,
        CancellationToken cancellationToken = default)
    {
        if (string.IsNullOrWhiteSpace(botToken))
        {
            return new TelegramChatDiscoveryResponse(false, null, null, "Bot token is required.");
        }

        try
        {
            var client = httpClientFactory.CreateClient();
            var url = $"https://api.telegram.org/bot{botToken}/getUpdates";
            using var response = await client.GetAsync(url, cancellationToken);
            if (!response.IsSuccessStatusCode)
            {
                return new TelegramChatDiscoveryResponse(
                    false,
                    null,
                    null,
                    $"Telegram API returned {(int)response.StatusCode} {response.ReasonPhrase}.");
            }

            await using var content = await response.Content.ReadAsStreamAsync(cancellationToken);
            using var doc = await JsonDocument.ParseAsync(content, cancellationToken: cancellationToken);
            var root = doc.RootElement;
            var ok = root.TryGetProperty("ok", out var okElement) && okElement.GetBoolean();
            if (!ok)
            {
                return new TelegramChatDiscoveryResponse(false, null, null, "Telegram API did not return ok=true.");
            }

            if (!root.TryGetProperty("result", out var resultElement) || resultElement.ValueKind != JsonValueKind.Array)
            {
                return new TelegramChatDiscoveryResponse(false, null, null, "Telegram API response did not include updates.");
            }

            for (var i = resultElement.GetArrayLength() - 1; i >= 0; i--)
            {
                if (!TryExtractChat(resultElement[i], out var chatId, out var chatTitle))
                {
                    continue;
                }

                return new TelegramChatDiscoveryResponse(true, chatId, chatTitle, null);
            }

            return new TelegramChatDiscoveryResponse(
                false,
                null,
                null,
                "No chats found yet. Send the bot a message and try again.");
        }
        catch (Exception ex)
        {
            return new TelegramChatDiscoveryResponse(false, null, null, ex.Message);
        }
    }

    public async Task SendAsync(SendNotificationRequest request, CancellationToken cancellationToken = default)
    {
        var providers = await db.NotificationProviders
            .Where(x => x.Enabled && request.ProviderTypes.Contains(x.Type))
            .ToListAsync(cancellationToken);
        foreach (var provider in providers)
        {
            switch (provider.Type)
            {
                case "smtp":
                    await SendSmtpAsync(provider, request.Subject, request.Body, cancellationToken);
                    break;
                case "telegram":
                    await SendTelegramAsync(provider, request.Subject, request.Body, cancellationToken);
                    break;
                case "discord":
                    await SendDiscordAsync(provider, request.Subject, request.Body, cancellationToken);
                    break;
            }
        }
    }

    private async Task SendSmtpAsync(NotificationProviderEntity provider, string subject, string body, CancellationToken cancellationToken)
    {
        using var doc = JsonDocument.Parse(provider.SettingsJson);
        var root = doc.RootElement;
        using var client = new SmtpClient(root.GetProperty("host").GetString())
        {
            Port = root.GetProperty("port").GetInt32(),
            EnableSsl = root.TryGetProperty("useTls", out var tls) && tls.GetBoolean(),
            Credentials = new NetworkCredential(
                root.GetProperty("username").GetString(),
                root.GetProperty("password").GetString()),
        };
        using var message = new MailMessage(
            root.GetProperty("from").GetString() ?? "hashi@localhost",
            root.GetProperty("to").GetString() ?? "admin@localhost",
            subject,
            body);
        await client.SendMailAsync(message, cancellationToken);
    }

    private async Task SendTelegramAsync(NotificationProviderEntity provider, string subject, string body, CancellationToken cancellationToken)
    {
        using var doc = JsonDocument.Parse(provider.SettingsJson);
        var root = doc.RootElement;
        var token = root.GetProperty("botToken").GetString();
        var chatId = root.GetProperty("chatId").GetString();
        var client = httpClientFactory.CreateClient();
        var url = $"https://api.telegram.org/bot{token}/sendMessage";
        var payload = new { chat_id = chatId, text = $"{subject}\n{body}" };
        using var response = await client.PostAsJsonAsync(url, payload, cancellationToken);
        response.EnsureSuccessStatusCode();
    }

    private async Task SendDiscordAsync(NotificationProviderEntity provider, string subject, string body, CancellationToken cancellationToken)
    {
        using var doc = JsonDocument.Parse(provider.SettingsJson);
        var webhook = doc.RootElement.GetProperty("webhookUrl").GetString();
        var client = httpClientFactory.CreateClient();
        var payload = new { content = $"{subject}\n{body}" };
        using var response = await client.PostAsJsonAsync(webhook!, payload, cancellationToken);
        response.EnsureSuccessStatusCode();
    }

    private static bool TryExtractChat(JsonElement update, out string chatId, out string chatTitle)
    {
        chatId = string.Empty;
        chatTitle = string.Empty;

        if (!TryGetChat(update, "message", out var chat)
            && !TryGetChat(update, "channel_post", out chat)
            && !TryGetChat(update, "my_chat_member", out chat)
            && !TryGetChat(update, "chat_member", out chat))
        {
            return false;
        }

        if (!chat.TryGetProperty("id", out var idElement))
        {
            return false;
        }

        chatId = idElement.ValueKind switch
        {
            JsonValueKind.Number when idElement.TryGetInt64(out var id) => id.ToString(),
            JsonValueKind.String => idElement.GetString() ?? string.Empty,
            _ => string.Empty,
        };
        if (string.IsNullOrWhiteSpace(chatId))
        {
            return false;
        }

        chatTitle = chat.TryGetProperty("title", out var title)
            ? title.GetString() ?? string.Empty
            : chat.TryGetProperty("username", out var username)
                ? username.GetString() ?? string.Empty
                : chat.TryGetProperty("first_name", out var firstName)
                    ? firstName.GetString() ?? string.Empty
                    : string.Empty;

        return true;
    }

    private static bool TryGetChat(JsonElement update, string nodeName, out JsonElement chat)
    {
        chat = default;
        if (!update.TryGetProperty(nodeName, out var node))
        {
            return false;
        }

        if (!node.TryGetProperty("chat", out chat))
        {
            return false;
        }

        return chat.ValueKind == JsonValueKind.Object;
    }
}
