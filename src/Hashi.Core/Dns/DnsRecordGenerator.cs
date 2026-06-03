using System.Net;
using Hashi.Core.Dns;

namespace Hashi.Core.Dns;

public sealed record FirewallHostDnsTarget(
    Guid Id,
    string Name,
    string? PublicIp,
    string? OnRouteTarget = null,
    IReadOnlyList<string>? ManagedSubnets = null,
    IReadOnlyList<string>? NetBirdRoutedCidrs = null,
    IReadOnlyList<string>? ConfiguredFqdns = null);

public sealed record PulseDnsTarget(
    Guid AgentId,
    string? PublicIp,
    string? InternalIp,
    string? Hostname = null);

public sealed record ResourceDnsTarget(
    string ResourceName,
    string Slug,
    string RootDomain,
    string? Domain,
    Guid? FirewallHostId,
    string? ManualIp,
    PulseDnsTarget? PulseTarget,
    string? ManualHost = null);

public static class DnsRecordGenerator
{
    public static IReadOnlyList<DnsRecordSnapshot> GenerateHostRecords(
        FirewallHostDnsTarget host,
        string rootDomain,
        int ttl = 3600)
    {
        if (string.IsNullOrWhiteSpace(host.PublicIp))
        {
            return [];
        }

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
        var resourceFqdn = ResolveResourceFqdn(target);
        var matchedHost = ResolveManagedHost(target, hosts);
        if (matchedHost is not null)
        {
            var onHost = $"on.{matchedHost.Name}.{target.RootDomain}".TrimEnd('.');
            return
            [
                new DnsRecordSnapshot(string.Empty, resourceFqdn, DnsRecordType.Cname, onHost, ttl, true),
            ];
        }

        var ip = FirstPublicIp(target.ManualIp, target.PulseTarget?.PublicIp);
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

        if (!string.IsNullOrWhiteSpace(target.ManualHost))
        {
            candidates.Add(target.ManualHost);
        }

        if (target.PulseTarget is not null)
        {
            candidates.Add(target.PulseTarget.PublicIp);
            candidates.Add(target.PulseTarget.InternalIp);
            candidates.Add(target.PulseTarget.Hostname);
        }

        foreach (var host in hosts)
        {
            foreach (var candidate in candidates.Where(c => !string.IsNullOrWhiteSpace(c)))
            {
                if (CandidateMatchesHost(candidate!, host))
                {
                    return host;
                }
            }
        }

        return null;
    }

    public static bool IsPublicIp(string? ipText)
        => IPAddress.TryParse(ipText, out var ip) && !IsPrivateOrSpecial(ip);

    public static string ResolveResourceFqdn(ResourceDnsTarget target)
    {
        var configuredDomain = target.Domain?.Trim().TrimEnd('.');
        if (!string.IsNullOrWhiteSpace(configuredDomain))
        {
            return configuredDomain == "@"
                ? target.RootDomain.TrimEnd('.')
                : configuredDomain;
        }

        return $"{target.Slug}.{target.RootDomain}".TrimEnd('.');
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

    private static bool CandidateMatchesHost(string candidate, FirewallHostDnsTarget host)
    {
        if (IPAddress.TryParse(candidate, out var candidateIp))
        {
            return IpTextEquals(candidateIp, host.PublicIp)
                || IpTextEquals(candidateIp, host.OnRouteTarget)
                || MatchesAnySubnet(candidate, host.ManagedSubnets)
                || MatchesAnySubnet(candidate, host.NetBirdRoutedCidrs);
        }

        var normalizedCandidate = NormalizeHost(candidate);
        if (string.IsNullOrWhiteSpace(normalizedCandidate))
        {
            return false;
        }

        return string.Equals(normalizedCandidate, NormalizeHost(host.OnRouteTarget), StringComparison.OrdinalIgnoreCase)
            || (host.ConfiguredFqdns is not null
                && host.ConfiguredFqdns.Any(fqdn =>
                    string.Equals(normalizedCandidate, NormalizeHost(fqdn), StringComparison.OrdinalIgnoreCase)));
    }

    private static bool IpTextEquals(IPAddress candidateIp, string? other)
        => IPAddress.TryParse(other, out var otherIp) && candidateIp.Equals(otherIp);

    private static bool MatchesAnySubnet(string candidate, IReadOnlyList<string>? cidrs)
        => cidrs is not null && cidrs.Any(subnet => IpMatchesSubnet(candidate, subnet));

    private static string? FirstPublicIp(params string?[] candidates)
        => candidates.FirstOrDefault(IsPublicIp);

    private static string NormalizeHost(string? host)
        => host?.Trim().TrimEnd('.').ToLowerInvariant() ?? string.Empty;

    private static bool IsPrivateOrSpecial(IPAddress ip)
        => IPAddress.IsLoopback(ip)
            || ip.Equals(IPAddress.Any)
            || ip.Equals(IPAddress.IPv6Any)
            || ip.IsIPv6LinkLocal
            || IsUniqueLocalIpv6(ip)
            || IsPrivateIpv4(ip);

    private static bool IsPrivateIpv4(IPAddress ip)
    {
        var bytes = ip.GetAddressBytes();
        return bytes.Length == 4
            && (bytes[0] == 10
                || (bytes[0] == 172 && bytes[1] >= 16 && bytes[1] <= 31)
                || (bytes[0] == 192 && bytes[1] == 168)
                || (bytes[0] == 169 && bytes[1] == 254));
    }

    private static bool IsUniqueLocalIpv6(IPAddress ip)
    {
        var bytes = ip.GetAddressBytes();
        return bytes.Length == 16 && (bytes[0] & 0xFE) == 0xFC;
    }
}
