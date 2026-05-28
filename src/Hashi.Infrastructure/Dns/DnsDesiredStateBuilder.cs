using System.Text.Json;
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
            .Where(x => x.Enabled && x.Domain != null)
            .ToListAsync(cancellationToken);
        var pulseAgents = await db.PulseAgents.AsNoTracking().ToDictionaryAsync(x => x.Id, cancellationToken);
        foreach (var resource in resources)
        {
            var slug = resource.Slug;
            PulseDnsTarget? pulseTarget = null;
            if (resource.PulseAgentId is Guid pulseId && pulseAgents.TryGetValue(pulseId, out var agent))
            {
                pulseTarget = new PulseDnsTarget(agent.Id, agent.LastPublicIp, agent.LastPrivateIp);
            }

            var records = DnsRecordGenerator.GenerateResourceRecords(
                new ResourceDnsTarget(
                    resource.Name,
                    slug,
                    rootDomain,
                    resource.Domain,
                    resource.FirewallHostId,
                    null,
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
        var merged = new Dictionary<string, DnsRecordSnapshot>(StringComparer.OrdinalIgnoreCase);
        foreach (var record in manual)
        {
            merged[Key(record)] = record;
        }

        foreach (var record in generated)
        {
            if (preservedManualNames.Contains(record.Name))
            {
                continue;
            }

            var conflicting = merged.Keys
                .Where(k => k.StartsWith($"{record.Name}|", StringComparison.OrdinalIgnoreCase))
                .ToList();
            foreach (var key in conflicting)
            {
                merged.Remove(key);
            }

            merged[Key(record)] = record with { IsManagedByHashi = true };
        }

        return merged.Values.ToList();
    }

    private static string Key(DnsRecordSnapshot record)
        => $"{record.Name}|{record.Type}";
}
