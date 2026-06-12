using System.Net;
using System.Net.Http.Json;
using System.Net.Mail;
using System.Text;
using System.Text.Json;
using System.Text.Json.Nodes;
using Hashi.Contracts.Api;
using Hashi.Core.Auth;
using Hashi.Infrastructure.Auth;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Infrastructure.Notifications;

public sealed class NotificationDispatcher(
    HashiDbContext db,
    IHttpClientFactory httpClientFactory,
    SecretRecordService secrets,
    IDiscordChannelDiscovery? discordDiscovery = null)
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
            SettingsJson = await BuildStoredSettingsJsonAsync(
                request.Type,
                request.Name,
                request.SettingsJson,
                existingSettingsJson: null,
                cancellationToken),
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
            entity.SettingsJson = await BuildStoredSettingsJsonAsync(
                entity.Type,
                entity.Name,
                request.SettingsJson,
                entity.SettingsJson,
                cancellationToken);
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

    public async Task<DiscordChannelDiscoveryResponse> DiscoverDiscordChannelAsync(
        string botToken,
        CancellationToken cancellationToken = default)
    {
        if (string.IsNullOrWhiteSpace(botToken))
        {
            return new DiscordChannelDiscoveryResponse(false, null, null, null, "Bot token is required.");
        }

        try
        {
            var discovery = discordDiscovery ?? new DiscordChannelDiscovery(httpClientFactory);
            var found = await discovery.DiscoverAsync(botToken, TimeSpan.FromSeconds(30), cancellationToken);
            return found is null
                ? new DiscordChannelDiscoveryResponse(false, null, null, null, "No DM or bot mention was received during pairing.")
                : new DiscordChannelDiscoveryResponse(true, found.ChannelId, found.ChannelName, found.UserId, null);
        }
        catch (OperationCanceledException) when (!cancellationToken.IsCancellationRequested)
        {
            return new DiscordChannelDiscoveryResponse(false, null, null, null, "Discord pairing timed out.");
        }
        catch (Exception ex)
        {
            return new DiscordChannelDiscoveryResponse(false, null, null, null, ex.Message);
        }
    }

    public async Task<IReadOnlyList<NotificationRouteResponse>> ListRoutesAsync(CancellationToken cancellationToken = default)
    {
        var routes = await db.NotificationRoutes.AsNoTracking()
            .OrderBy(x => x.Name)
            .ToListAsync(cancellationToken);
        return routes.Select(ToRouteResponse).ToList();
    }

    public async Task<NotificationRouteResponse> CreateRouteAsync(
        CreateNotificationRouteRequest request,
        CancellationToken cancellationToken = default)
    {
        if (!await db.NotificationProviders.AnyAsync(x => x.Id == request.ProviderId, cancellationToken))
        {
            throw new InvalidOperationException("Notification provider not found.");
        }

        var entity = new NotificationRouteEntity
        {
            ProviderId = request.ProviderId,
            Name = NormalizeRouteName(request.Name),
            EventKind = NormalizeEventKind(request.EventKind),
            Severity = NormalizeSeverity(request.Severity),
            MatchJson = NormalizeMatchJson(request.MatchJson),
            Enabled = request.Enabled,
            CooldownMinutes = NormalizeCooldown(request.CooldownMinutes),
            SendRecovery = request.SendRecovery,
        };
        db.NotificationRoutes.Add(entity);
        await db.SaveChangesAsync(cancellationToken);
        return ToRouteResponse(entity);
    }

    public async Task<NotificationRouteResponse?> UpdateRouteAsync(
        Guid routeId,
        UpdateNotificationRouteRequest request,
        CancellationToken cancellationToken = default)
    {
        var entity = await db.NotificationRoutes.SingleOrDefaultAsync(x => x.Id == routeId, cancellationToken);
        if (entity is null)
        {
            return null;
        }

        if (request.ProviderId is Guid providerId)
        {
            if (!await db.NotificationProviders.AnyAsync(x => x.Id == providerId, cancellationToken))
            {
                throw new InvalidOperationException("Notification provider not found.");
            }

            entity.ProviderId = providerId;
        }

        if (request.Name is not null)
            entity.Name = NormalizeRouteName(request.Name);
        if (request.EventKind is not null)
            entity.EventKind = NormalizeEventKind(request.EventKind);
        if (request.Severity is not null)
            entity.Severity = NormalizeSeverity(request.Severity);
        if (request.MatchJson is not null)
            entity.MatchJson = NormalizeMatchJson(request.MatchJson);
        if (request.Enabled is bool enabled)
            entity.Enabled = enabled;
        if (request.CooldownMinutes is int cooldown)
            entity.CooldownMinutes = NormalizeCooldown(cooldown);
        if (request.SendRecovery is bool sendRecovery)
            entity.SendRecovery = sendRecovery;

        await db.SaveChangesAsync(cancellationToken);
        return ToRouteResponse(entity);
    }

    public async Task<bool> DeleteRouteAsync(Guid routeId, CancellationToken cancellationToken = default)
    {
        var entity = await db.NotificationRoutes.SingleOrDefaultAsync(x => x.Id == routeId, cancellationToken);
        if (entity is null)
        {
            return false;
        }

        db.NotificationRoutes.Remove(entity);
        await db.SaveChangesAsync(cancellationToken);
        return true;
    }

    public async Task SendByProviderAsync(
        NotificationProviderEntity provider,
        Guid? routeId,
        string eventKind,
        string subject,
        string body,
        CancellationToken cancellationToken = default)
    {
        var delivery = new NotificationDeliveryEntity
        {
            RouteId = routeId,
            ProviderId = provider.Id,
            EventKind = eventKind,
            Subject = subject,
            Status = NotificationDeliveryStatusNames.Pending,
            AttemptCount = 1,
        };
        db.NotificationDeliveries.Add(delivery);

        try
        {
            switch (provider.Type)
            {
                case "smtp":
                    await SendSmtpAsync(provider, subject, body, cancellationToken);
                    break;
                case "telegram":
                    await SendTelegramAsync(provider, subject, body, cancellationToken);
                    break;
                case "discord":
                    await SendDiscordAsync(provider, subject, body, cancellationToken);
                    break;
                default:
                    delivery.Status = NotificationDeliveryStatusNames.Failed;
                    delivery.ErrorDetails = $"Unsupported provider type: {provider.Type}";
                    await db.SaveChangesAsync(cancellationToken);
                    return;
            }

            delivery.Status = NotificationDeliveryStatusNames.Sent;
            delivery.SentAtUtc = DateTimeOffset.UtcNow;
        }
        catch (Exception ex)
        {
            delivery.Status = NotificationDeliveryStatusNames.Failed;
            delivery.ErrorDetails = ex.Message;
        }

        await db.SaveChangesAsync(cancellationToken);
    }

    public async Task SendAsync(SendNotificationRequest request, CancellationToken cancellationToken = default)
    {
        var providers = await db.NotificationProviders
            .Where(x => x.Enabled && request.ProviderTypes.Contains(x.Type))
            .ToListAsync(cancellationToken);
        foreach (var provider in providers)
        {
            var delivery = new NotificationDeliveryEntity
            {
                ProviderId = provider.Id,
                EventKind = "notification",
                Subject = request.Subject,
                Status = NotificationDeliveryStatusNames.Pending,
                AttemptCount = 1,
            };
            db.NotificationDeliveries.Add(delivery);

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
                        delivery.Status = NotificationDeliveryStatusNames.Failed;
                        delivery.ErrorDetails = $"Unsupported provider type: {provider.Type}";
                        await db.SaveChangesAsync(cancellationToken);
                        continue;
                }

                delivery.Status = NotificationDeliveryStatusNames.Sent;
                delivery.SentAtUtc = DateTimeOffset.UtcNow;
            }
            catch (Exception ex)
            {
                delivery.Status = NotificationDeliveryStatusNames.Failed;
                delivery.ErrorDetails = ex.Message;
                await db.SaveChangesAsync(cancellationToken);
                throw;
            }

            await db.SaveChangesAsync(cancellationToken);
        }
    }

    private async Task SendSmtpAsync(NotificationProviderEntity provider, string subject, string body, CancellationToken cancellationToken)
    {
        using var doc = JsonDocument.Parse(provider.SettingsJson);
        var root = doc.RootElement;
        var password = await ResolveSecretStringAsync(root, "passwordSecretId", "SMTP password", cancellationToken);
        using var client = new SmtpClient(root.GetProperty("host").GetString())
        {
            Port = root.GetProperty("port").GetInt32(),
            EnableSsl = root.TryGetProperty("useTls", out var tls) && tls.GetBoolean(),
            Credentials = new NetworkCredential(
                root.GetProperty("username").GetString(),
                password),
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
        var token = await ResolveSecretStringAsync(root, "botTokenSecretId", "Telegram bot token", cancellationToken);
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
        var client = httpClientFactory.CreateClient();
        var payload = new { content = $"{subject}\n{body}" };
        HttpResponseMessage response;
        if (doc.RootElement.TryGetProperty("botTokenSecretId", out _) &&
            doc.RootElement.TryGetProperty("channelId", out var channelIdElement))
        {
            var token = await ResolveSecretStringAsync(doc.RootElement, "botTokenSecretId", "Discord bot token", cancellationToken);
            using var request = new HttpRequestMessage(
                HttpMethod.Post,
                $"https://discord.com/api/v10/channels/{Uri.EscapeDataString(channelIdElement.GetString() ?? string.Empty)}/messages")
            {
                Content = JsonContent.Create(payload),
            };
            request.Headers.Authorization = new System.Net.Http.Headers.AuthenticationHeaderValue("Bot", token);
            response = await client.SendAsync(request, cancellationToken);
        }
        else
        {
            var webhook = await ResolveSecretStringAsync(doc.RootElement, "webhookSecretId", "Discord webhook URL", cancellationToken);
            response = await client.PostAsJsonAsync(webhook, payload, cancellationToken);
        }
        using (response)
        {
            response.EnsureSuccessStatusCode();
        }
    }

    private async Task<string> BuildStoredSettingsJsonAsync(
        string providerType,
        string providerName,
        string settingsJson,
        string? existingSettingsJson,
        CancellationToken cancellationToken)
    {
        var settings = ParseSettingsObject(settingsJson);
        var existing = string.IsNullOrWhiteSpace(existingSettingsJson)
            ? null
            : ParseSettingsObject(existingSettingsJson);

        switch (providerType)
        {
            case "smtp":
                await MoveSecretToVaultAsync(
                    settings,
                    existing,
                    plaintextProperty: "password",
                    secretIdProperty: "passwordSecretId",
                    label: $"Notification SMTP password: {providerName}",
                    cancellationToken);
                break;
            case "telegram":
                await MoveSecretToVaultAsync(
                    settings,
                    existing,
                    plaintextProperty: "botToken",
                    secretIdProperty: "botTokenSecretId",
                    label: $"Notification Telegram token: {providerName}",
                    cancellationToken);
                break;
            case "discord":
                await MoveSecretToVaultAsync(
                    settings,
                    existing,
                    plaintextProperty: "botToken",
                    secretIdProperty: "botTokenSecretId",
                    label: $"Notification Discord bot token: {providerName}",
                    cancellationToken);
                await MoveSecretToVaultAsync(
                    settings,
                    existing,
                    plaintextProperty: "webhookUrl",
                    secretIdProperty: "webhookSecretId",
                    label: $"Notification Discord webhook: {providerName}",
                    cancellationToken);
                break;
        }

        return settings.ToJsonString();
    }

    private async Task MoveSecretToVaultAsync(
        JsonObject settings,
        JsonObject? existing,
        string plaintextProperty,
        string secretIdProperty,
        string label,
        CancellationToken cancellationToken)
    {
        var plaintext = ReadString(settings, plaintextProperty);
        settings.Remove(plaintextProperty);
        settings.Remove(secretIdProperty);

        if (!string.IsNullOrWhiteSpace(plaintext))
        {
            var secret = await secrets.StoreAsync(
                SecretPurpose.NotificationToken,
                label,
                Encoding.UTF8.GetBytes(plaintext),
                cancellationToken,
                serviceSyncEligible: RuntimeSecretEligibility.IsRuntimePurpose(SecretPurpose.NotificationToken));
            settings[secretIdProperty] = secret.Id.ToString();
            return;
        }

        var existingSecretId = existing is null ? null : ReadString(existing, secretIdProperty);
        if (!string.IsNullOrWhiteSpace(existingSecretId))
        {
            settings[secretIdProperty] = existingSecretId;
        }
    }

    private async Task<string> ResolveSecretStringAsync(
        JsonElement settings,
        string secretIdProperty,
        string label,
        CancellationToken cancellationToken)
    {
        if (!settings.TryGetProperty(secretIdProperty, out var secretIdElement)
            || !Guid.TryParse(secretIdElement.GetString(), out var secretId))
        {
            throw new InvalidOperationException($"{label} is not configured.");
        }

        var plaintext = await secrets.DecryptForPurposeAsync(secretId, cancellationToken)
            ?? throw new InvalidOperationException($"{label} unavailable; unlock vault or configure service-sync vault.");
        return Encoding.UTF8.GetString(plaintext);
    }

    private static JsonObject ParseSettingsObject(string settingsJson)
    {
        var node = JsonNode.Parse(settingsJson);
        return node as JsonObject ?? throw new InvalidOperationException("Notification provider settings must be a JSON object.");
    }

    private static string? ReadString(JsonObject settings, string property)
        => settings.TryGetPropertyValue(property, out var node) ? node?.GetValue<string>() : null;

    private static string NormalizeRouteName(string name)
    {
        var normalized = name.Trim();
        if (string.IsNullOrWhiteSpace(normalized))
        {
            throw new InvalidOperationException("Notification route name is required.");
        }

        return normalized;
    }

    private static string NormalizeEventKind(string eventKind)
    {
        var normalized = eventKind.Trim().ToLowerInvariant();
        return normalized switch
        {
            "all" or "monitor" or "security" => normalized,
            _ => throw new InvalidOperationException("Notification route event kind must be all, monitor, or security."),
        };
    }

    private static string NormalizeSeverity(string severity)
    {
        var normalized = severity.Trim().ToLowerInvariant();
        return normalized switch
        {
            "info" or "warning" or "critical" => normalized,
            _ => throw new InvalidOperationException("Notification route severity must be info, warning, or critical."),
        };
    }

    private static string NormalizeMatchJson(string matchJson)
    {
        if (string.IsNullOrWhiteSpace(matchJson))
        {
            return "{}";
        }

        try
        {
            using var doc = JsonDocument.Parse(matchJson);
            if (doc.RootElement.ValueKind != JsonValueKind.Object)
            {
                throw new InvalidOperationException("Notification route match JSON must be a JSON object.");
            }

            return doc.RootElement.GetRawText();
        }
        catch (JsonException ex)
        {
            throw new InvalidOperationException("Notification route match JSON is invalid.", ex);
        }
    }

    private static int NormalizeCooldown(int cooldownMinutes)
    {
        if (cooldownMinutes < 0)
        {
            throw new InvalidOperationException("Notification route cooldown must be zero or greater.");
        }

        return cooldownMinutes;
    }

    private static NotificationRouteResponse ToRouteResponse(NotificationRouteEntity entity)
        => new(
            entity.Id,
            entity.ProviderId,
            entity.Name,
            entity.EventKind,
            entity.Severity,
            entity.MatchJson,
            entity.Enabled,
            entity.CooldownMinutes,
            entity.SendRecovery,
            entity.CreatedAtUtc);

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
