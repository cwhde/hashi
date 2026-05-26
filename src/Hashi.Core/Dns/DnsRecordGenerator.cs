namespace Hashi.Core.Dns;

public sealed record FirewallHostDnsTarget(
    string Name,
    string PublicIp,
    string? OnRouteTarget = null);

public sealed record PulseDnsTarget(
    Guid AgentId,
    string? PublicIp,
    string? InternalIp);

public sealed record ResourceDnsTarget(
    string ResourceName,
    string Slug,
    string RootDomain,
    Guid? FirewallHostId,
    string? ManualIp,
    PulseDnsTarget? PulseTarget);

public static class DnsRecordGenerator
{
    public static IReadOnlyList<DnsRecordSnapshot> GenerateHostRecords(
        FirewallHostDnsTarget host,
        string rootDomain,
        int ttl = 3600)
    {
        var fqdn = $"{host.Name}.{rootDomain}".TrimEnd('.');
        var via = $"via.{host.Name}.{rootDomain}".TrimEnd('.');
        var on = $"on.{host.Name}.{rootDomain}".TrimEnd('.');
        var onTarget = string.IsNullOrWhiteSpace(host.OnRouteTarget)
            ? via
            : host.OnRouteTarget.TrimEnd('.');

        return
        [
            new DnsRecordSnapshot(string.Empty, fqdn, DnsRecordType.A, host.PublicIp, ttl, true),
            new DnsRecordSnapshot(string.Empty, via, DnsRecordType.Cname, fqdn, ttl, true),
            new DnsRecordSnapshot(string.Empty, on, DnsRecordType.Cname, onTarget, ttl, true),
        ];
    }

    public static IReadOnlyList<DnsRecordSnapshot> GenerateResourceRecords(
        ResourceDnsTarget target,
        IReadOnlyList<FirewallHostDnsTarget> hosts,
        int ttl = 3600)
    {
        var resourceFqdn = $"{target.Slug}.{target.RootDomain}".TrimEnd('.');
        var matchedHost = ResolveManagedHost(target, hosts);
        if (matchedHost is not null)
        {
            var onHost = $"on.{matchedHost.Name}.{target.RootDomain}".TrimEnd('.');
            return
            [
                new DnsRecordSnapshot(string.Empty, resourceFqdn, DnsRecordType.Cname, onHost, ttl, true),
            ];
        }

        var ip = target.ManualIp ?? target.PulseTarget?.PublicIp;
        if (string.IsNullOrWhiteSpace(ip))
        {
            return [];
        }

        var recordType = ip.Contains(':') ? DnsRecordType.Aaaa : DnsRecordType.A;
        return
        [
            new DnsRecordSnapshot(string.Empty, resourceFqdn, recordType, ip, ttl, true),
        ];
    }

    public static FirewallHostDnsTarget? ResolveManagedHost(
        ResourceDnsTarget target,
        IReadOnlyList<FirewallHostDnsTarget> hosts)
    {
        if (target.FirewallHostId is Guid hostId)
        {
            return hosts.FirstOrDefault(h => h.Name == hostId.ToString()) ?? hosts.FirstOrDefault();
        }

        var candidates = new List<string?>();
        if (!string.IsNullOrWhiteSpace(target.ManualIp))
        {
            candidates.Add(target.ManualIp);
        }

        if (target.PulseTarget is not null)
        {
            candidates.Add(target.PulseTarget.PublicIp);
            candidates.Add(target.PulseTarget.InternalIp);
        }

        foreach (var host in hosts)
        {
            if (candidates.Any(c => string.Equals(c, host.PublicIp, StringComparison.OrdinalIgnoreCase)))
            {
                return host;
            }
        }

        return null;
    }
}
