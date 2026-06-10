using System.Text.Json;
using Hashi.Infrastructure.Notifications;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Infrastructure.Platform;

public sealed class NotificationRoutingService(
    HashiDbContext db,
    NotificationDispatcher dispatcher)
{
    public async Task RouteMonitorTransitionAsync(
        MonitorEndpointEntity endpoint,
        string previousStatus,
        string newStatus,
        CancellationToken cancellationToken = default)
    {
        if (string.Equals(previousStatus, newStatus, StringComparison.OrdinalIgnoreCase))
        {
            return;
        }

        var isDown = string.Equals(newStatus, "down", StringComparison.OrdinalIgnoreCase);
        var isRecovery = (string.Equals(previousStatus, "down", StringComparison.OrdinalIgnoreCase)
                || string.Equals(previousStatus, "degraded", StringComparison.OrdinalIgnoreCase))
            && string.Equals(newStatus, "up", StringComparison.OrdinalIgnoreCase);

        if (!isDown && !isRecovery)
        {
            return;
        }

        var eventKind = "monitor";
        var severity = isDown ? "critical" : "info";
        var subject = isDown
            ? $"Monitor down: {endpoint.Name}"
            : $"Monitor recovered: {endpoint.Name}";
        var body = isDown
            ? $"{endpoint.Name} ({endpoint.Url}) is down."
            : $"{endpoint.Name} ({endpoint.Url}) is back up.";

        var matchProperties = new Dictionary<string, string>
        {
            ["name"] = endpoint.Name,
            ["url"] = endpoint.Url,
        };
        if (endpoint.ResourceId.HasValue)
        {
            matchProperties["resourceId"] = endpoint.ResourceId.Value.ToString();
        }

        await RouteEventAsync(eventKind, severity, subject, body, isRecovery, matchProperties, cancellationToken);
    }

    public async Task RouteSecurityEventAsync(
        string subject,
        string body,
        CancellationToken cancellationToken = default)
    {
        var matchProperties = new Dictionary<string, string>
        {
            ["subject"] = subject,
        };

        await RouteEventAsync("security", "critical", subject, body, isRecovery: false, matchProperties, cancellationToken);
    }

    private async Task RouteEventAsync(
        string eventKind,
        string severity,
        string subject,
        string body,
        bool isRecovery,
        Dictionary<string, string> matchProperties,
        CancellationToken cancellationToken)
    {
        var routes = await db.NotificationRoutes
            .AsNoTracking()
            .Include(x => x.Provider)
            .Where(x => x.Enabled && x.Provider.Enabled)
            .ToListAsync(cancellationToken);

        foreach (var route in routes)
        {
            if (!MatchesEventKind(route, eventKind))
            {
                continue;
            }

            if (!MeetsSeverityThreshold(route, severity))
            {
                continue;
            }

            if (!MatchesEndpoint(route, matchProperties))
            {
                continue;
            }

            if (isRecovery && !route.SendRecovery)
            {
                continue;
            }

            if (await IsCooldownActiveAsync(route, subject, cancellationToken))
            {
                continue;
            }

            await dispatcher.SendByProviderAsync(
                route.Provider,
                route.Id,
                eventKind,
                subject,
                body,
                cancellationToken);
        }
    }

    private static bool MatchesEventKind(NotificationRouteEntity route, string eventKind)
        => route.EventKind.Equals("all", StringComparison.OrdinalIgnoreCase)
            || route.EventKind.Equals(eventKind, StringComparison.OrdinalIgnoreCase);

    private static bool MeetsSeverityThreshold(NotificationRouteEntity route, string eventSeverity)
    {
        var routeLevel = SeverityLevel(route.Severity);
        var eventLevel = SeverityLevel(eventSeverity);
        return eventLevel >= routeLevel;
    }

    private static int SeverityLevel(string severity) => severity.ToLowerInvariant() switch
    {
        "critical" => 3,
        "warning" => 2,
        "info" => 1,
        _ => 0,
    };

    private static bool MatchesEndpoint(NotificationRouteEntity route, Dictionary<string, string> matchProperties)
    {
        if (string.IsNullOrWhiteSpace(route.MatchJson) || route.MatchJson == "{}")
        {
            return true;
        }

        try
        {
            using var doc = JsonDocument.Parse(route.MatchJson);
            var root = doc.RootElement;
            if (root.ValueKind != JsonValueKind.Object)
            {
                return false;
            }

            foreach (var property in root.EnumerateObject())
            {
                if (property.Value.ValueKind != JsonValueKind.String)
                {
                    return false;
                }

                if (!matchProperties.TryGetValue(property.Name, out var actual)
                    || !string.Equals(property.Value.GetString(), actual, StringComparison.OrdinalIgnoreCase))
                {
                    return false;
                }
            }

            return true;
        }
        catch (JsonException)
        {
            return false;
        }
    }

    private async Task<bool> IsCooldownActiveAsync(NotificationRouteEntity route, string subject, CancellationToken cancellationToken)
    {
        if (route.CooldownMinutes <= 0)
        {
            return false;
        }

        var cooldownThreshold = DateTimeOffset.UtcNow.AddMinutes(-route.CooldownMinutes);
        var recentDelivery = await db.NotificationDeliveries
            .AsNoTracking()
            .AnyAsync(x =>
                x.RouteId == route.Id
                && x.Subject == subject
                && x.Status == NotificationDeliveryStatusNames.Sent
                && (x.SentAtUtc ?? x.CreatedAtUtc) >= cooldownThreshold, cancellationToken);
        return recentDelivery;
    }
}
