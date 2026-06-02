using System.Text.Json;
using System.Net;
using Hashi.Core.Dns;
using Hashi.Core.Resources;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
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
                null,
                JsonSerializer.Deserialize<List<string>>(h.ManagedSubnetsJson) ?? []))
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
            var slug = resource.Slug;
            PulseDnsTarget? pulseTarget = null;
            if (resource.PulseAgentId is Guid pulseId && pulseAgents.TryGetValue(pulseId, out var agent))
            {
                pulseTarget = new PulseDnsTarget(agent.Id, agent.LastPublicIp, agent.LastSelectedIp ?? agent.LastPrivateIp);
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
                    ResolveManualPublicIp(resource.TargetHost),
                    pulseTarget),
                hostTargets,
                defaultTtl);
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

    private static string? ResolveManualPublicIp(string? targetHost)
    {
        if (!IPAddress.TryParse(targetHost, out var ip))
        {
            return null;
        }

        if (IPAddress.IsLoopback(ip)
            || ip.IsIPv6LinkLocal
            || ip.IsIPv6SiteLocal
            || IsPrivateIpv4(ip))
        {
            return null;
        }

        return ip.ToString();
    }

    private static bool IsPrivateIpv4(IPAddress ip)
    {
        var bytes = ip.GetAddressBytes();
        return bytes.Length == 4
            && (bytes[0] == 10
                || (bytes[0] == 172 && bytes[1] >= 16 && bytes[1] <= 31)
                || (bytes[0] == 192 && bytes[1] == 168));
    }
}
