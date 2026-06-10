using System.Text.Json;
using System.Net;
using Hashi.Core.Dns;
using Hashi.Core.Resources;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Platform;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Infrastructure.Dns;

public static class DnsDesiredStateBuilder
{
    public static async Task<IReadOnlyList<DnsRecordSnapshot>> BuildAsync(
        HashiDbContext db,
        Guid zoneId,
        int defaultTtl,
        CancellationToken cancellationToken = default)
    {
        var manual = await db.DnsRecords.AsNoTracking()
            .Where(x => x.ZoneId == zoneId && x.Enabled)
            .ToListAsync(cancellationToken);
        var manualSnapshots = manual
            .Select(x => new DnsRecordSnapshot(
                x.ProviderRecordId,
                x.Name,
                DnsRecordTypeMapping.Parse(x.Type),
                x.Value,
                x.Ttl ?? defaultTtl,
                true))
            .ToList();

        var settings = await db.AppSettings.AsNoTracking().SingleOrDefaultAsync(cancellationToken);
        var rootDomain = settings?.RootDomain;
        if (string.IsNullOrWhiteSpace(rootDomain))
        {
            return manualSnapshots;
        }

        var generated = new List<DnsRecordSnapshot>();
        var hosts = await db.FirewallHosts.AsNoTracking().ToListAsync(cancellationToken);
        var hostTargets = hosts
            .Select(h => new FirewallHostDnsTarget(
                h.Id,
                h.Name,
                h.PublicIp,
                ResolveOnRouteTarget(h),
                DeserializeStringList(h.ManagedSubnetsJson),
                DeserializeStringList(h.NetBirdRoutedCidrsJson),
                BuildConfiguredFqdns(h, rootDomain)))
            .ToList();

        foreach (var host in hostTargets)
        {
            generated.AddRange(DnsRecordGenerator.GenerateHostRecords(host, rootDomain, defaultTtl));
        }

        var resources = await db.Resources.AsNoTracking()
            .Where(x => x.Enabled)
            .ToListAsync(cancellationToken);
        var pulseAgents = await db.PulseAgents.AsNoTracking().ToDictionaryAsync(x => x.Id, cancellationToken);
        foreach (var resource in resources)
        {
            if (resource.FirewallHostId is null && resource.DetectedFirewallHostId is null)
            {
                AutoDetectFirewallHost(resource, hostTargets);
            }

            var slug = resource.Slug;
            PulseDnsTarget? pulseTarget = null;
            if (resource.PulseAgentId is Guid pulseId && pulseAgents.TryGetValue(pulseId, out var agent))
            {
                pulseTarget = new PulseDnsTarget(
                    agent.Id,
                    agent.LastPublicIp,
                    agent.LastSelectedIp ?? agent.LastPrivateIp,
                    agent.LastHostname);
            }

            var resolvedDomain = ResourceDomainResolver.Resolve(
                resource.DomainMode,
                resource.Domain,
                resource.Slug,
                rootDomain);
            if (string.IsNullOrWhiteSpace(resolvedDomain))
            {
                continue;
            }

            var records = DnsRecordGenerator.GenerateResourceRecords(
                new ResourceDnsTarget(
                    resource.Name,
                    slug,
                    rootDomain,
                    resolvedDomain,
                    resource.FirewallHostId,
                    ResolveManualIp(resource.TargetHost),
                    pulseTarget,
                    ResolveManualHost(resource.TargetHost)),
                hostTargets,
                defaultTtl);
            if (records.Count == 0 && IsPrivateManualIp(resource.TargetHost))
            {
                throw new InvalidOperationException(
                    $"Resource '{resource.Name}' targets private address '{resource.TargetHost}', but it does not match any managed firewall host subnet, NetBird routed CIDR, or configured host FQDN. Select a firewall host or add the target range to managed topology before DNS sync.");
            }

            generated.AddRange(records);
        }

        return MergeRecords(manual, generated);
    }

    internal static IReadOnlyList<DnsRecordSnapshot> MergeRecords(
        IReadOnlyList<DnsRecordEntity> manual,
        IReadOnlyList<DnsRecordSnapshot> generated)
    {
        var preserveNames = manual
            .Where(x => x.Ownership is DnsOwnershipNames.User or DnsOwnershipNames.Imported)
            .Select(x => x.Name)
            .ToHashSet(StringComparer.OrdinalIgnoreCase);
        var snapshots = manual
            .Select(x => new DnsRecordSnapshot(
                x.ProviderRecordId,
                x.Name,
                DnsRecordTypeMapping.Parse(x.Type),
                x.Value,
                x.Ttl,
                true))
            .ToList();

        return MergeRecords(snapshots, generated, preserveNames);
    }

    internal static IReadOnlyList<DnsRecordSnapshot> MergeRecords(
        IReadOnlyList<DnsRecordSnapshot> manual,
        IReadOnlyList<DnsRecordSnapshot> generated)
        => MergeRecords(manual, generated, manual.Select(x => x.Name).ToHashSet(StringComparer.OrdinalIgnoreCase));

    private static IReadOnlyList<DnsRecordSnapshot> MergeRecords(
        IReadOnlyList<DnsRecordSnapshot> manual,
        IReadOnlyList<DnsRecordSnapshot> generated,
        HashSet<string> preservedManualNames)
    {
        var merged = new List<DnsRecordSnapshot>();
        foreach (var record in manual)
        {
            merged.RemoveAll(x => string.Equals(Key(x), Key(record), StringComparison.OrdinalIgnoreCase));
            merged.Add(record);
        }

        foreach (var record in generated)
        {
            var managed = record with { IsManagedByHashi = true };
            if (preservedManualNames.Contains(record.Name)
                && manual.Any(x => string.Equals(Key(x), Key(record), StringComparison.OrdinalIgnoreCase)))
            {
                merged.Add(managed);
                continue;
            }

            merged.RemoveAll(x => !IsPreservedManual(x, preservedManualNames)
                && string.Equals(Key(x), Key(record), StringComparison.OrdinalIgnoreCase));
            merged.Add(managed);
        }

        return merged;
    }

    private static string Key(DnsRecordSnapshot record)
    {
        var key = $"{record.Name}|{record.Type}";
        return IsMultiValue(record.Type)
            ? $"{key}|{record.Value.Trim().TrimEnd('.')}"
            : key;
    }

    private static bool IsPreservedManual(DnsRecordSnapshot record, HashSet<string> preservedManualNames)
        => preservedManualNames.Contains(record.Name);

    private static bool IsMultiValue(DnsRecordType type)
        => type is DnsRecordType.Mx or DnsRecordType.Txt;

    private static string? ResolveManualIp(string? targetHost)
    {
        if (!IPAddress.TryParse(targetHost, out var ip))
        {
            return null;
        }

        return ip.ToString();
    }

    private static string? ResolveManualHost(string? targetHost)
        => !string.IsNullOrWhiteSpace(targetHost) && !IPAddress.TryParse(targetHost, out _)
            ? targetHost.Trim()
            : null;

    private static bool IsPrivateManualIp(string? targetHost)
        => ResolveManualIp(targetHost) is string ip && !DnsRecordGenerator.IsPublicIp(ip);

    private static string? ResolveOnRouteTarget(FirewallHostEntity host)
    {
        if (!string.IsNullOrWhiteSpace(host.LinkedTraefikHost))
        {
            return host.LinkedTraefikHost.Trim();
        }

        if (!string.IsNullOrWhiteSpace(host.InternalTraefikIp)
            && System.Net.IPAddress.TryParse(host.InternalTraefikIp.Trim(), out _))
        {
            return null;
        }

        return host.InternalTraefikIp?.Trim();
    }

    private static IReadOnlyList<string> BuildConfiguredFqdns(FirewallHostEntity host, string rootDomain)
    {
        var fqdns = new HashSet<string>(StringComparer.OrdinalIgnoreCase);
        Add(host.Domain);
        Add(FirewallTrustedIpResolver.BuildFqdn(host));
        Add($"{host.Name}.{rootDomain}");
        Add($"via.{host.Name}.{rootDomain}");
        Add($"on.{host.Name}.{rootDomain}");
        return fqdns.ToList();

        void Add(string? value)
        {
            if (!string.IsNullOrWhiteSpace(value))
            {
                fqdns.Add(value.Trim().TrimEnd('.'));
            }
        }
    }

    private static IReadOnlyList<string> DeserializeStringList(string json)
        => JsonSerializer.Deserialize<List<string>>(json) ?? [];

    private static void AutoDetectFirewallHost(ResourceEntity resource, IReadOnlyList<FirewallHostDnsTarget> hosts)
    {
        var candidates = new List<string?>();
        if (!string.IsNullOrWhiteSpace(resource.TargetHost))
        {
            candidates.Add(resource.TargetHost.Trim());
        }

        foreach (var host in hosts)
        {
            foreach (var candidate in candidates.Where(c => !string.IsNullOrWhiteSpace(c)))
            {
                if ((host.ManagedSubnets?.Any(subnet => DnsRecordGenerator.IpMatchesSubnet(candidate!, subnet)) ?? false)
                    || (host.NetBirdRoutedCidrs?.Any(cidr => DnsRecordGenerator.IpMatchesSubnet(candidate!, cidr)) ?? false))
                {
                    resource.DetectedFirewallHostId = host.Id;
                    return;
                }
            }
        }
    }
}
