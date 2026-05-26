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
}
