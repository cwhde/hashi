using Hashi.Contracts.Api;
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

        var providerTypes = await GetEnabledProviderTypesAsync(cancellationToken);
        if (providerTypes.Count == 0)
        {
            return;
        }

        if (string.Equals(newStatus, "down", StringComparison.OrdinalIgnoreCase))
        {
            await dispatcher.SendAsync(new SendNotificationRequest(
                $"Monitor down: {endpoint.Name}",
                $"{endpoint.Name} ({endpoint.Url}) is down.",
                providerTypes), cancellationToken);
        }
        else if (string.Equals(previousStatus, "down", StringComparison.OrdinalIgnoreCase)
            && string.Equals(newStatus, "up", StringComparison.OrdinalIgnoreCase))
        {
            await dispatcher.SendAsync(new SendNotificationRequest(
                $"Monitor recovered: {endpoint.Name}",
                $"{endpoint.Name} ({endpoint.Url}) is back up.",
                providerTypes), cancellationToken);
        }
    }

    public async Task RouteSecurityEventAsync(
        string subject,
        string body,
        CancellationToken cancellationToken = default)
    {
        var providerTypes = await GetEnabledProviderTypesAsync(cancellationToken);
        if (providerTypes.Count == 0)
        {
            return;
        }

        await dispatcher.SendAsync(new SendNotificationRequest(subject, body, providerTypes), cancellationToken);
    }

    private async Task<IReadOnlyList<string>> GetEnabledProviderTypesAsync(CancellationToken cancellationToken)
        => await db.NotificationProviders.AsNoTracking()
            .Where(x => x.Enabled)
            .Select(x => x.Type)
            .Distinct()
            .ToListAsync(cancellationToken);
}
