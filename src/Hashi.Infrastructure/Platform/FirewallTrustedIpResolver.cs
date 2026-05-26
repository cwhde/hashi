using System.Net;
using System.Net.Sockets;
using Hashi.Infrastructure.Persistence.Entities;
using Microsoft.Extensions.Logging;

namespace Hashi.Infrastructure.Platform;

public sealed class FirewallTrustedIpResolver(ILogger<FirewallTrustedIpResolver> logger)
{
    public async Task<IReadOnlyList<string>> ResolveTrustedPublicIpsAsync(
        IReadOnlyList<FirewallHostEntity> hosts,
        CancellationToken cancellationToken = default)
    {
        var ips = new HashSet<string>(StringComparer.Ordinal);
        foreach (var host in hosts)
        {
            if (!string.IsNullOrWhiteSpace(host.PublicIp))
            {
                ips.Add(host.PublicIp.Trim());
                continue;
            }

            var fqdn = BuildFqdn(host);
            if (string.IsNullOrWhiteSpace(fqdn))
            {
                continue;
            }

            try
            {
                var addresses = await System.Net.Dns.GetHostAddressesAsync(fqdn, cancellationToken);
                foreach (var address in addresses.Where(x => x.AddressFamily == System.Net.Sockets.AddressFamily.InterNetwork))
                {
                    ips.Add(address.ToString());
                }
            }
            catch (Exception ex) when (ex is SocketException or ArgumentException)
            {
                logger.LogDebug(ex, "Could not resolve trusted public IP for firewall host {Host} ({Fqdn})", host.Name, fqdn);
            }
        }

        return ips.ToList();
    }

    public static string BuildFqdn(FirewallHostEntity host)
    {
        if (host.Domain.Contains('.', StringComparison.Ordinal))
        {
            return host.Domain;
        }

        if (string.IsNullOrWhiteSpace(host.Name) || string.IsNullOrWhiteSpace(host.Domain))
        {
            return host.Domain;
        }

        return $"{host.Name}.{host.Domain}";
    }
}
