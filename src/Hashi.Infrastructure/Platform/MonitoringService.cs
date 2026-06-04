using Hashi.Contracts.Api;
using Hashi.Core.Resources;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Services;
using Microsoft.EntityFrameworkCore;
using System.Text.Json;

namespace Hashi.Infrastructure.Platform;

public sealed class MonitoringService(HashiDbContext db, AppSettingsService settings, HashiInternalUrlResolver internalUrls)
{
    public async Task SyncEndpointsFromResourcesAsync(CancellationToken cancellationToken = default)
    {
        var resources = await db.Resources.AsNoTracking()
            .Where(x => x.Enabled && x.StatusEnabled)
            .ToListAsync(cancellationToken);
        var existing = await db.MonitorEndpoints.ToListAsync(cancellationToken);

        foreach (var resource in resources)
        {
            var checkType = ResolveResourceCheckType(resource);
            UpsertProvisionedEndpoint(
                existing,
                resourceId: resource.Id,
                dnsRecordId: null,
                resource.Name,
                BuildResourceMonitorUrl(resource, checkType),
                checkType,
                enabled: true);
        }

        foreach (var monitor in existing.Where(x => x.ResourceId is not null))
        {
            if (resources.All(r => r.Id != monitor.ResourceId))
            {
                monitor.Enabled = false;
            }
        }

        await SyncInfrastructureEndpointsAsync(existing, cancellationToken);
        await db.SaveChangesAsync(cancellationToken);
    }

    public async Task RecordTransitionAsync(
        MonitorEndpointEntity endpoint,
        string previousStatus,
        string newStatus,
        int? latencyMs,
        CancellationToken cancellationToken = default)
    {
        if (string.Equals(previousStatus, newStatus, StringComparison.OrdinalIgnoreCase))
        {
            return;
        }

        db.MonitorEvents.Add(new MonitorEventEntity
        {
            MonitorEndpointId = endpoint.Id,
            PreviousStatus = previousStatus,
            NewStatus = newStatus,
            LatencyMs = latencyMs,
        });
        await db.SaveChangesAsync(cancellationToken);
    }

    public async Task<IReadOnlyList<MonitorEndpointEntity>> ListAsync(CancellationToken cancellationToken = default)
        => await db.MonitorEndpoints.AsNoTracking().OrderBy(x => x.Name).ToListAsync(cancellationToken);

    public async Task<IReadOnlyList<MonitorEndpointResponse>> ListResponsesAsync(CancellationToken cancellationToken = default)
    {
        var endpoints = await db.MonitorEndpoints.AsNoTracking().OrderBy(x => x.Name).ToListAsync(cancellationToken);
        var resourceIds = endpoints
            .Where(x => x.ResourceId is not null)
            .Select(x => x.ResourceId!.Value)
            .Distinct()
            .ToList();
        var resources = await db.Resources.AsNoTracking()
            .Where(x => resourceIds.Contains(x.Id))
            .ToDictionaryAsync(x => x.Id, cancellationToken);
        var firewallIds = resources.Values
            .Where(x => x.FirewallHostId is not null)
            .Select(x => x.FirewallHostId!.Value)
            .Distinct()
            .ToList();
        var firewallHosts = await db.FirewallHosts.AsNoTracking()
            .Where(x => firewallIds.Contains(x.Id))
            .ToDictionaryAsync(x => x.Id, cancellationToken);
        var allFirewallHosts = await db.FirewallHosts.AsNoTracking().ToListAsync(cancellationToken);

        return endpoints
            .Select(endpoint => ToResponse(endpoint, BuildMetadata(endpoint, resources, firewallHosts, allFirewallHosts)))
            .ToList();
    }

    public async Task<MonitorEndpointEntity> CreateManualAsync(
        CreateMonitorEndpointRequest request,
        CancellationToken cancellationToken = default)
    {
        var endpoint = new MonitorEndpointEntity
        {
            Name = RequireName(request.Name),
            Url = RequireUrl(request.Url),
            CheckType = NormalizeManualCheckType(request.CheckType),
            Enabled = request.Enabled,
            PublicStatusEnabled = request.PublicStatusEnabled,
        };
        db.MonitorEndpoints.Add(endpoint);
        await db.SaveChangesAsync(cancellationToken);
        return endpoint;
    }

    public async Task<MonitorEndpointEntity?> UpdateManualAsync(
        Guid endpointId,
        UpdateMonitorEndpointRequest request,
        CancellationToken cancellationToken = default)
    {
        var endpoint = await db.MonitorEndpoints.SingleOrDefaultAsync(x => x.Id == endpointId, cancellationToken);
        if (endpoint is null)
        {
            return null;
        }

        var updatesManagedFields = request.Name is not null
            || request.Url is not null
            || request.CheckType is not null
            || request.Enabled is not null;
        if ((endpoint.ResourceId is not null || endpoint.DnsRecordId is not null) && updatesManagedFields)
        {
            throw new InvalidOperationException("Provisioned monitor endpoints are managed by their source.");
        }

        if (request.Name is not null)
        {
            endpoint.Name = RequireName(request.Name);
        }

        if (request.Url is not null)
        {
            endpoint.Url = RequireUrl(request.Url);
        }

        if (request.CheckType is not null)
        {
            endpoint.CheckType = NormalizeManualCheckType(request.CheckType);
        }

        if (request.Enabled is bool enabled)
        {
            endpoint.Enabled = enabled;
        }

        if (request.PublicStatusEnabled is bool publicStatusEnabled)
        {
            endpoint.PublicStatusEnabled = publicStatusEnabled;
        }

        await db.SaveChangesAsync(cancellationToken);
        return endpoint;
    }

    public async Task<bool> DeleteManualAsync(Guid endpointId, CancellationToken cancellationToken = default)
    {
        var endpoint = await db.MonitorEndpoints.SingleOrDefaultAsync(x => x.Id == endpointId, cancellationToken);
        if (endpoint is null)
        {
            return false;
        }

        if (endpoint.ResourceId is not null || endpoint.DnsRecordId is not null)
        {
            throw new InvalidOperationException("Provisioned monitor endpoints are managed by their source.");
        }

        db.MonitorEndpoints.Remove(endpoint);
        await db.SaveChangesAsync(cancellationToken);
        return true;
    }

    public async Task<IReadOnlyList<MonitorEventEntity>> ListEventsAsync(
        Guid? endpointId,
        int hours,
        CancellationToken cancellationToken = default)
    {
        var since = DateTimeOffset.UtcNow.AddHours(-Math.Clamp(hours, 1, 720));
        var query = db.MonitorEvents.AsNoTracking().Where(x => x.OccurredAtUtc >= since);
        if (endpointId is Guid id)
        {
            query = query.Where(x => x.MonitorEndpointId == id);
        }

        return await query.OrderByDescending(x => x.OccurredAtUtc).Take(500).ToListAsync(cancellationToken);
    }

    public async Task<IReadOnlyList<MonitorRollupEntity>> ListRollupsAsync(
        Guid? endpointId,
        int? intervalMinutes,
        int hours,
        CancellationToken cancellationToken = default)
    {
        var since = DateTimeOffset.UtcNow.AddHours(-Math.Clamp(hours, 1, 720));
        var query = db.MonitorRollups.AsNoTracking().Where(x => x.BucketStartUtc >= since);
        if (endpointId is Guid id)
        {
            query = query.Where(x => x.MonitorEndpointId == id);
        }

        if (intervalMinutes is int interval && interval > 0)
        {
            query = query.Where(x => x.IntervalMinutes == interval);
        }

        return await query.OrderBy(x => x.BucketStartUtc).ToListAsync(cancellationToken);
    }

    public async Task<IReadOnlyList<PublicStatusItemResponse>> PublicStatusAsync(CancellationToken cancellationToken = default)
    {
        var hours = 1;
        var endpoints = await db.MonitorEndpoints.AsNoTracking()
            .Where(x => x.Enabled && x.PublicStatusEnabled)
            .OrderBy(x => x.Name)
            .ToListAsync(cancellationToken);
        var since = DateTimeOffset.UtcNow.AddHours(-hours);
        var rollups = await db.MonitorRollups.AsNoTracking()
            .Where(x => x.IntervalMinutes == 1 && x.BucketStartUtc >= since)
            .OrderBy(x => x.BucketStartUtc)
            .ToListAsync(cancellationToken);
        var pendingPulseIds = await db.PulseAgents.AsNoTracking()
            .Where(x => x.DnsPendingAtUtc != null && x.DnsPendingAtUtc > DateTimeOffset.UtcNow.AddHours(-2))
            .Select(x => x.Id)
            .ToListAsync(cancellationToken);
        var pendingResourceIds = await db.Resources.AsNoTracking()
            .Where(x => x.PulseAgentId != null && pendingPulseIds.Contains(x.PulseAgentId.Value))
            .Select(x => x.Id)
            .ToHashSetAsync(cancellationToken);

        return endpoints.Select(endpoint =>
        {
            var strip = rollups
                .Where(x => x.MonitorEndpointId == endpoint.Id)
                .Select(x => new PublicStatusStripBucket(
                    x.BucketStartUtc,
                    x.UpCount >= x.DownCount))
                .ToList();
            var status = endpoint.ResourceId is not null && pendingResourceIds.Contains(endpoint.ResourceId.Value)
                ? "Pending"
                : NormalizeStatus(endpoint.Status);

            return new PublicStatusItemResponse(
                endpoint.Name,
                status,
                endpoint.LastLatencyMs,
                strip);
        }).ToList();
    }

    public async Task<PublicStatusSummaryResponse> PublicSummaryAsync(CancellationToken cancellationToken = default)
    {
        var endpoints = await db.MonitorEndpoints.AsNoTracking()
            .Where(x => x.Enabled && x.PublicStatusEnabled)
            .ToListAsync(cancellationToken);
        var hosts = await db.FirewallHosts.AsNoTracking().ToListAsync(cancellationToken);
        var up = endpoints.Count(x => x.Status.Equals("up", StringComparison.OrdinalIgnoreCase));
        var down = endpoints.Count(x => x.Status.Equals("down", StringComparison.OrdinalIgnoreCase));
        var degraded = endpoints.Count(x => x.Status.Equals("degraded", StringComparison.OrdinalIgnoreCase));
        return new PublicStatusSummaryResponse(
            endpoints.Count,
            up,
            degraded,
            down,
            hosts.Count,
            hosts.Count(h => h.LastAppliedAtUtc is not null));
    }

    public static MonitorEndpointResponse ToResponse(MonitorEndpointEntity entity) => ToResponse(entity, null);

    private static MonitorEndpointResponse ToResponse(
        MonitorEndpointEntity entity,
        MonitorEndpointMetadata? metadata) => new(
        entity.Id,
        entity.Name,
        entity.Url,
        entity.CheckType,
        entity.Enabled,
        entity.PublicStatusEnabled,
        NormalizeStatus(entity.Status),
        entity.LastCheckedAtUtc,
        entity.LastLatencyMs,
        entity.ResourceId,
        metadata?.ResourceType,
        metadata?.Host,
        metadata?.FirewallHostId,
        metadata?.FirewallHostName,
        entity.ResourceId is not null);

    public async Task<bool> IsPublicStatusEnabledAsync(CancellationToken cancellationToken = default)
    {
        var appSettings = await settings.GetOrCreateAsync(cancellationToken);
        return appSettings.PublicStatusEnabled;
    }

    public static MonitorRollupResponse ToRollupResponse(MonitorRollupEntity entity) => new(
        entity.MonitorEndpointId,
        entity.BucketStartUtc,
        entity.IntervalMinutes,
        entity.SampleCount,
        entity.UpCount,
        entity.DownCount,
        entity.AverageLatencyMs);

    public static MonitorEventResponse ToEventResponse(MonitorEventEntity entity) => new(
        entity.Id,
        entity.MonitorEndpointId,
        entity.PreviousStatus,
        entity.NewStatus,
        entity.LatencyMs,
        entity.OccurredAtUtc);

    private static string NormalizeStatus(string status) => status.ToLowerInvariant() switch
    {
        "up" => "Up",
        "down" => "Down",
        "degraded" => "Degraded",
        "paused" => "Paused",
        _ => "Unknown",
    };

    private static MonitorEndpointMetadata BuildMetadata(
        MonitorEndpointEntity endpoint,
        IReadOnlyDictionary<Guid, ResourceEntity> resources,
        IReadOnlyDictionary<Guid, FirewallHostEntity> firewallHosts,
        IReadOnlyList<FirewallHostEntity> allFirewallHosts)
    {
        if (endpoint.ResourceId is Guid resourceId && resources.TryGetValue(resourceId, out var resource))
        {
            FirewallHostEntity? firewallHost = null;
            if (resource.FirewallHostId is Guid firewallHostId)
            {
                firewallHosts.TryGetValue(firewallHostId, out firewallHost);
            }

            return new MonitorEndpointMetadata(
                resource.Kind,
                FirstNonEmpty(resource.Domain, resource.TargetHost, TryReadHost(endpoint.Url)),
                resource.FirewallHostId,
                firewallHost?.Name);
        }

        var matchedFirewallHost = allFirewallHosts.FirstOrDefault(host =>
            endpoint.Name.Equals($"Firewall: {host.Name}", StringComparison.OrdinalIgnoreCase));
        return new MonitorEndpointMetadata(
            "Infrastructure",
            TryReadHost(endpoint.Url),
            matchedFirewallHost?.Id,
            matchedFirewallHost?.Name);
    }

    private static string? TryReadHost(string url)
    {
        if (Uri.TryCreate(url, UriKind.Absolute, out var uri) && !string.IsNullOrWhiteSpace(uri.Host))
        {
            return uri.Host;
        }

        return null;
    }

    private sealed record MonitorEndpointMetadata(
        string? ResourceType,
        string? Host,
        Guid? FirewallHostId,
        string? FirewallHostName);

    private async Task SyncInfrastructureEndpointsAsync(
        List<MonitorEndpointEntity> existing,
        CancellationToken cancellationToken)
    {
        var appSettings = await settings.GetOrCreateAsync(cancellationToken);
        UpsertProvisionedEndpoint(
            existing,
            resourceId: null,
            dnsRecordId: null,
            "Hashi API",
            internalUrls.ResolveUrl(appSettings, "/api/health"),
            "http",
            enabled: true);

        var firewallHosts = await db.FirewallHosts.AsNoTracking().ToListAsync(cancellationToken);
        foreach (var host in firewallHosts)
        {
            var target = FirstNonEmpty(host.PublicIp, host.Domain, host.LinkedTraefikHost, host.InternalTraefikIp);
            if (target is null)
            {
                continue;
            }

            UpsertProvisionedEndpoint(
                existing,
                resourceId: null,
                dnsRecordId: null,
                $"Firewall: {host.Name}",
                $"icmp://{target}",
                "icmp",
                enabled: true);
        }

        var connections = await db.Connections.AsNoTracking()
            .Where(x => x.Enabled && (x.Type == ConnectionTypeNames.TraefikHost || x.Type == ConnectionTypeNames.FirewallHost))
            .ToListAsync(cancellationToken);
        foreach (var connection in connections.Where(x => x.Type == ConnectionTypeNames.TraefikHost))
        {
            if (!TryReadSshTarget(connection.SettingsJson, out var host, out var port))
            {
                continue;
            }

            UpsertProvisionedEndpoint(
                existing,
                resourceId: null,
                dnsRecordId: null,
                $"Traefik SSH: {connection.Name}",
                $"tcp://{host}:{port}",
                "tcp",
                enabled: true);
        }

        var adGuardConnections = await db.AdGuardConnections.AsNoTracking()
            .Where(x => x.Enabled)
            .ToListAsync(cancellationToken);
        foreach (var connection in adGuardConnections)
        {
            UpsertProvisionedEndpoint(
                existing,
                resourceId: null,
                dnsRecordId: null,
                $"AdGuard: {connection.Name}",
                connection.BaseUrl,
                "http",
                enabled: true);
        }

        var manualDnsRecords = await db.DnsRecords.AsNoTracking()
            .Where(x => x.Enabled
                && x.Ownership == DnsOwnershipNames.User
                && x.MonitoringEnabled
                && x.MonitoringDisplayName != null)
            .ToListAsync(cancellationToken);
        foreach (var record in manualDnsRecords)
        {
            var monitorName = record.MonitoringDisplayName!.Trim();
            if (monitorName.Length == 0)
            {
                continue;
            }

            UpsertProvisionedEndpoint(
                existing,
                resourceId: null,
                dnsRecordId: record.Id,
                monitorName,
                $"dns://{record.Name}",
                "dns",
                enabled: record.Enabled);
        }

        DisableOrphanedDnsMonitorEndpoints(existing, manualDnsRecords);

        var pulseAgents = await db.PulseAgents.AsNoTracking().ToListAsync(cancellationToken);
        foreach (var agent in pulseAgents)
        {
            var target = FirstNonEmpty(agent.LastPublicIp, agent.LastSelectedIp, agent.LastPrivateIp, agent.LastHostname);
            if (target is null)
            {
                continue;
            }

            UpsertProvisionedEndpoint(
                existing,
                resourceId: null,
                dnsRecordId: null,
                $"Pulse network: {agent.Name}",
                $"icmp://{target}",
                "icmp",
                enabled: agent.Status != "revoked");
        }
    }

    private MonitorEndpointEntity UpsertProvisionedEndpoint(
        List<MonitorEndpointEntity> existing,
        Guid? resourceId,
        Guid? dnsRecordId,
        string name,
        string url,
        string checkType,
        bool enabled)
    {
        var monitor = resourceId is Guid resourceIdValue
            ? existing.SingleOrDefault(x => x.ResourceId == resourceIdValue)
            : dnsRecordId is Guid dnsRecordIdValue
                ? existing.SingleOrDefault(x => x.DnsRecordId == dnsRecordIdValue)
                : existing.SingleOrDefault(x => x.ResourceId == null && x.DnsRecordId == null && x.Name == name);
        if (monitor is null)
        {
            monitor = new MonitorEndpointEntity { ResourceId = resourceId, DnsRecordId = dnsRecordId };
            db.MonitorEndpoints.Add(monitor);
            existing.Add(monitor);
        }

        monitor.ResourceId = resourceId;
        monitor.DnsRecordId = dnsRecordId;
        monitor.Name = name;
        monitor.Url = url;
        monitor.CheckType = NormalizeManualCheckType(checkType);
        monitor.Enabled = enabled;
        return monitor;
    }

    private static void DisableOrphanedDnsMonitorEndpoints(
        List<MonitorEndpointEntity> existing,
        IReadOnlyCollection<DnsRecordEntity> monitoredRecords)
    {
        var monitoredRecordIds = monitoredRecords.Select(x => x.Id).ToHashSet();
        foreach (var monitor in existing.Where(m =>
                     m.DnsRecordId is Guid id
                     && !monitoredRecordIds.Contains(id)))
        {
            monitor.Enabled = false;
        }
    }

    private static string ResolveResourceCheckType(ResourceEntity resource)
    {
        if (!string.IsNullOrWhiteSpace(resource.MonitoringProtocolHint))
        {
            return NormalizeManualCheckType(resource.MonitoringProtocolHint);
        }

        var kind = (resource.Kind ?? string.Empty).Trim().ToLowerInvariant();
        return kind switch
        {
            "http" => "http",
            "https" => "https",
            "h2c" => "h2c",
            "tcp" => "tcp",
            "udp" => "udp",
            "pulse" or "push" => "pulse",
            _ when resource.TargetScheme.Equals("h2c", StringComparison.OrdinalIgnoreCase) => "h2c",
            _ when resource.TargetScheme.Equals("http", StringComparison.OrdinalIgnoreCase) => "http",
            _ => "https",
        };
    }

    private static string BuildResourceMonitorUrl(ResourceEntity resource, string checkType)
    {
        var domainOrHost = FirstNonEmpty(resource.Domain, resource.TargetHost) ?? "127.0.0.1";
        return checkType switch
        {
            "http" => $"http://{domainOrHost}/",
            "https" => $"https://{domainOrHost}/",
            "h2c" => $"http://{domainOrHost}/",
            "tcp" => $"tcp://{resource.TargetHost}:{resource.PublicPort ?? resource.TargetPort}",
            "udp" => $"udp://{resource.TargetHost}:{resource.PublicPort ?? resource.TargetPort}",
            "dns" => $"dns://{domainOrHost}",
            "icmp" => $"icmp://{domainOrHost}",
            "tls" => $"tls://{domainOrHost}:{resource.PublicPort ?? resource.TargetPort}",
            "pulse" => $"pulse://{resource.Id}",
            _ => $"{resource.TargetScheme}://{domainOrHost}/",
        };
    }

    private static string NormalizeManualCheckType(string checkType)
    {
        var normalized = ResourceMonitoringProtocolHintNames.Normalize(checkType) ?? string.Empty;

        if (!ResourceMonitoringProtocolHintNames.IsValid(normalized))
        {
            throw new InvalidOperationException($"Unsupported monitor check type '{checkType}'.");
        }

        return normalized;
    }

    private static string RequireName(string name)
    {
        var trimmed = (name ?? string.Empty).Trim();
        return string.IsNullOrWhiteSpace(trimmed)
            ? throw new InvalidOperationException("Monitor endpoint name is required.")
            : trimmed;
    }

    private static string RequireUrl(string url)
    {
        var trimmed = (url ?? string.Empty).Trim();
        return string.IsNullOrWhiteSpace(trimmed)
            ? throw new InvalidOperationException("Monitor endpoint URL is required.")
            : trimmed;
    }

    private static string? FirstNonEmpty(params string?[] values)
        => values.FirstOrDefault(x => !string.IsNullOrWhiteSpace(x))?.Trim();

    private static bool TryReadSshTarget(string settingsJson, out string host, out int port)
    {
        host = string.Empty;
        port = 22;
        if (string.IsNullOrWhiteSpace(settingsJson))
        {
            return false;
        }

        try
        {
            using var doc = JsonDocument.Parse(settingsJson);
            var root = doc.RootElement;
            if (root.TryGetProperty("Host", out var hostProperty) || root.TryGetProperty("host", out hostProperty))
            {
                host = hostProperty.GetString() ?? string.Empty;
            }

            if (root.TryGetProperty("Port", out var portProperty) || root.TryGetProperty("port", out portProperty))
            {
                port = portProperty.GetInt32();
            }
        }
        catch (JsonException)
        {
            return false;
        }

        return !string.IsNullOrWhiteSpace(host) && port > 0;
    }
}
