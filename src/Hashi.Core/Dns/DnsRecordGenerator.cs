using System.Net;
using Hashi.Core.Dns;

namespace Hashi.Core.Dns;

public sealed record FirewallHostDnsTarget(
    Guid Id,
    string Name,
    string PublicIp,
    string? OnRouteTarget = null,
    IReadOnlyList<string>? ManagedSubnets = null);

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
            return hosts.FirstOrDefault(h => h.Id == hostId);
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
            foreach (var candidate in candidates.Where(c => !string.IsNullOrWhiteSpace(c)))
            {
                if (string.Equals(candidate, host.PublicIp, StringComparison.OrdinalIgnoreCase))
                {
                    return host;
                }

                if (host.ManagedSubnets is not null
                    && host.ManagedSubnets.Any(subnet => IpMatchesSubnet(candidate!, subnet)))
                {
                    return host;
                }
            }
        }

        return null;
    }

    public static bool IpMatchesSubnet(string ipText, string cidr)
    {
        if (!IPAddress.TryParse(ipText, out var ip) || !cidr.Contains('/'))
        {
            return false;
        }

        var parts = cidr.Split('/');
        if (!IPAddress.TryParse(parts[0], out var network) || !int.TryParse(parts[1], out var prefixLength))
        {
            return false;
        }

        var ipBytes = ip.GetAddressBytes();
        var networkBytes = network.GetAddressBytes();
        if (ipBytes.Length != networkBytes.Length)
        {
            return false;
        }

        var fullBytes = prefixLength / 8;
        var remainingBits = prefixLength % 8;
        for (var i = 0; i < fullBytes; i++)
        {
            if (ipBytes[i] != networkBytes[i])
            {
                return false;
            }
        }

        if (remainingBits == 0)
        {
            return true;
        }

        var mask = (byte)(0xFF << (8 - remainingBits));
        return (ipBytes[fullBytes] & mask) == (networkBytes[fullBytes] & mask);
    }
}
