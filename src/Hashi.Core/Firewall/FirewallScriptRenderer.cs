namespace Hashi.Core.Firewall;

public sealed record FirewallHostDefinition(
    Guid Id,
    string Name,
    string Domain,
    IReadOnlyList<string> ManagedSubnets,
    string LinkedTraefikHost,
    string InternalTraefikIp);

public static class FirewallScriptRenderer
{
    public static string Render(FirewallHostDefinition host)
    {
        var subnets = string.Join(' ', host.ManagedSubnets);
        return $$"""
            #!/bin/bash
            set -euo pipefail
            # Hashi-managed firewall script for {{host.Name}}
            ipset create hashi_trusted hash:net family inet hashsize 1024 maxelem 65536 -exist
            ipset create hashi_blocked hash:net family inet hashsize 1024 maxelem 65536 -exist
            iptables -N HASHI_INPUT -exist || true
            iptables -A INPUT -j HASHI_INPUT
            for subnet in {{subnets}}; do
              iptables -A HASHI_INPUT -s "$subnet" -j ACCEPT
            done
            """;
    }
}
