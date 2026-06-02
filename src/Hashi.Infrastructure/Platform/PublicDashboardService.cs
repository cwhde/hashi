using Hashi.Contracts.Api;
using Hashi.Core.Resources;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Services;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Infrastructure.Platform;

public sealed class PublicDashboardService(HashiDbContext db, AppSettingsService settings)
{
    public async Task<bool> IsPublicDashboardEnabledAsync(CancellationToken cancellationToken = default)
    {
        var appSettings = await settings.GetOrCreateAsync(cancellationToken);
        return appSettings.PublicDashboardEnabled;
    }

    public async Task<PublicDashboardResponse> GetAsync(CancellationToken cancellationToken = default)
    {
        var appSettings = await settings.GetOrCreateAsync(cancellationToken);
        var rootDomain = appSettings.RootDomain;
        var resources = await db.Resources.AsNoTracking()
            .Where(x => x.DashboardEnabled)
            .OrderBy(x => x.Name)
            .ToListAsync(cancellationToken);
        var manualDnsRecords = await db.DnsRecords.AsNoTracking()
            .Where(x => x.Enabled
                && x.Ownership == DnsOwnershipNames.User
                && x.DashboardEnabled
                && x.DashboardDisplayName != null
                && x.DashboardDisplayName != "")
            .OrderBy(x => x.DashboardDisplayName)
            .ToListAsync(cancellationToken);
        var monitors = await db.MonitorEndpoints.AsNoTracking()
            .Where(x => x.Enabled)
            .ToListAsync(cancellationToken);
        var firewallHosts = await db.FirewallHosts.AsNoTracking().ToListAsync(cancellationToken);

        var items = new List<PublicDashboardItemResponse>();
        foreach (var resource in resources)
        {
            var resolvedDomain = ResourceDomainResolver.Resolve(resource.DomainMode, resource.Domain, resource.Slug, rootDomain);
            var publicUrl = BuildResourcePublicUrl(resource, resolvedDomain);
            if (publicUrl is null)
            {
                continue;
            }

            var monitor = monitors.FirstOrDefault(x => x.ResourceId == resource.Id);
            items.Add(new PublicDashboardItemResponse(
                resource.Id,
                "resource",
                resource.Name,
                publicUrl,
                resolvedDomain,
                ResolveStatus(monitor?.Status, resource.Enabled),
                monitor?.LastLatencyMs));
        }

        foreach (var record in manualDnsRecords)
        {
            var monitor = monitors.FirstOrDefault(x =>
                x.ResourceId is null
                && x.Name.Equals($"DNS: {record.Name}", StringComparison.OrdinalIgnoreCase));
            items.Add(new PublicDashboardItemResponse(
                record.Id,
                "manual_dns",
                record.DashboardDisplayName!,
                $"https://{record.Name}",
                record.Name,
                ResolveStatus(monitor?.Status, record.Enabled),
                monitor?.LastLatencyMs));
        }

        var orderedItems = items
            .OrderBy(x => x.DisplayName, StringComparer.OrdinalIgnoreCase)
            .ToList();
        return new PublicDashboardResponse(
            orderedItems,
            orderedItems.Count(x => x.Status.Equals("Online", StringComparison.OrdinalIgnoreCase)),
            orderedItems.Count,
            firewallHosts.Count(x => x.LastAppliedAtUtc is not null),
            firewallHosts.Count);
    }

    private static string? BuildResourcePublicUrl(ResourceEntity resource, string? resolvedDomain)
    {
        var domain = NormalizeOptional(resolvedDomain);
        if (domain is null)
        {
            return null;
        }

        var pathPrefix = NormalizePathPrefix(resource.PathPrefix);
        return resource.Kind.Trim().ToLowerInvariant() switch
        {
            "http" => $"http://{domain}{pathPrefix}",
            "h2c" => $"http://{domain}{pathPrefix}",
            "tcp" => $"tcp://{domain}:{resource.PublicPort ?? resource.TargetPort}",
            "udp" => $"udp://{domain}:{resource.PublicPort ?? resource.TargetPort}",
            _ => $"https://{domain}{pathPrefix}",
        };
    }

    private static string? NormalizeOptional(string? value)
    {
        var normalized = value?.Trim();
        return string.IsNullOrWhiteSpace(normalized) ? null : normalized;
    }

    private static string NormalizePathPrefix(string? value)
    {
        var normalized = NormalizeOptional(value);
        if (normalized is null || normalized == "/")
        {
            return string.Empty;
        }

        return normalized.StartsWith("/", StringComparison.Ordinal) ? normalized : $"/{normalized}";
    }

    private static string ResolveStatus(string? monitorStatus, bool enabled)
    {
        if (!enabled)
        {
            return "Offline";
        }

        return monitorStatus?.Trim().ToLowerInvariant() switch
        {
            "up" => "Online",
            "down" => "Offline",
            "degraded" => "Degraded",
            "pending" => "Pending",
            _ => "Online",
        };
    }
}
