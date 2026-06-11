using System.Text.RegularExpressions;

namespace Hashi.Core.Firewall;

public sealed record FirewallHostDefinition(
    Guid Id,
    string Name,
    string Domain,
    IReadOnlyList<string> ManagedSubnets,
    string LinkedTraefikHost,
    string InternalTraefikIp,
    string? PublicIp = null,
    string? WanInterface = null,
    string? LxcBridge = null,
    bool NetBirdEnabled = true,
    string NetBirdInterface = "wt0",
    IReadOnlyList<string>? NetBirdOverlayCidrs = null,
    IReadOnlyList<string>? NetBirdRoutedCidrs = null,
    bool NetBirdRoutingPeer = false,
    IReadOnlyList<FirewallPortForward>? PortForwards = null,
    IReadOnlyList<string>? TrustedPublicIps = null,
    IReadOnlyList<string>? BlockedIps = null,
    int RollbackTimerSeconds = 300);

public sealed record FirewallPortForward(
    string Protocol,
    int PublicPort,
    string TargetHost,
    int TargetPort);

public static class FirewallScriptRenderer
{
    private static readonly Regex ShellMetacharPattern = new(
        @"[;|&`$(){}!\n\r\\\""'#<>]",
        RegexOptions.Compiled);


    private static readonly Regex ValidNamePattern = new(
        @"^[a-zA-Z0-9._-]+$",
        RegexOptions.Compiled);

    private static readonly Regex ValidIpPattern = new(
        @"^[a-zA-Z0-9.:/_ -]+$",
        RegexOptions.Compiled);

    private static readonly Regex ValidInterfacePattern = new(
        @"^[a-zA-Z0-9._-]+$",
        RegexOptions.Compiled);

    private static readonly Regex ValidProtocolPattern = new(
        @"^(tcp|udp|icmp|sctp)$",
        RegexOptions.Compiled | RegexOptions.IgnoreCase);

    private static readonly Regex ValidCidrPattern = new(
        @"^[a-zA-Z0-9.:/_-]+$",
        RegexOptions.Compiled);

    public static string ShellEscape(string value)
    {
        ArgumentException.ThrowIfNullOrWhiteSpace(value);
        return "'" + value.Replace("'", "'\\''", StringComparison.Ordinal) + "'";
    }

    private static void ValidateHostDefinition(FirewallHostDefinition host)
    {
        ValidateStringField(host.Name, nameof(host.Name), ValidNamePattern);
        ValidateStringField(host.Domain, nameof(host.Domain), ValidNamePattern);
        ValidateStringField(host.InternalTraefikIp, nameof(host.InternalTraefikIp), ValidIpPattern);
        ValidateStringField(host.NetBirdInterface, nameof(host.NetBirdInterface), ValidInterfacePattern);

        if (host.PublicIp is not null)
        {
            ValidateStringField(host.PublicIp, nameof(host.PublicIp), ValidIpPattern);
        }

        if (host.WanInterface is not null)
        {
            ValidateStringField(host.WanInterface, nameof(host.WanInterface), ValidInterfacePattern);
        }

        if (host.LxcBridge is not null)
        {
            ValidateStringField(host.LxcBridge, nameof(host.LxcBridge), ValidInterfacePattern);
        }

        foreach (var subnet in host.ManagedSubnets)
        {
            ValidateStringField(subnet, "ManagedSubnet", ValidCidrPattern);
        }

        foreach (var ip in host.TrustedPublicIps ?? [])
        {
            ValidateStringField(ip, "TrustedPublicIp", ValidIpPattern);
        }

        foreach (var ip in host.BlockedIps ?? [])
        {
            ValidateStringField(ip, "BlockedIp", ValidIpPattern);
        }

        foreach (var cidr in host.NetBirdOverlayCidrs ?? [])
        {
            ValidateStringField(cidr, "NetBirdOverlayCidr", ValidCidrPattern);
        }

        foreach (var cidr in host.NetBirdRoutedCidrs ?? [])
        {
            ValidateStringField(cidr, "NetBirdRoutedCidr", ValidCidrPattern);
        }

        foreach (var pf in host.PortForwards ?? [])
        {
            ValidateStringField(pf.Protocol, nameof(FirewallPortForward.Protocol), ValidProtocolPattern);
            ValidateStringField(pf.TargetHost, nameof(FirewallPortForward.TargetHost), ValidIpPattern);
        }
    }

    private static void ValidateStringField(string value, string fieldName, Regex pattern)
    {
        ArgumentException.ThrowIfNullOrWhiteSpace(value);
        if (!pattern.IsMatch(value))
        {
            throw new ArgumentException($"Field '{fieldName}' contains invalid characters: {value}", fieldName);
        }
    }

    public static string Render(FirewallHostDefinition host)
    {
        ValidateHostDefinition(host);

        var managedSubnetsV4Array = RenderBashArray("HASHI_MANAGED_SUBNETS", host.ManagedSubnets.Where(x => !IsIpv6(x)));
        var managedSubnetsV6Array = RenderBashArray("HASHI_MANAGED_SUBNETS6", host.ManagedSubnets.Where(IsIpv6));
        var overlayCidrs = host.NetBirdOverlayCidrs ?? ["100.110.0.0/16"];
        var routedCidrs = host.NetBirdRoutedCidrs ?? [];
        var overlayCidrsV4Array = RenderBashArray("HASHI_NETBIRD_OVERLAY", overlayCidrs.Where(x => !IsIpv6(x)));
        var overlayCidrsV6Array = RenderBashArray("HASHI_NETBIRD_OVERLAY6", overlayCidrs.Where(IsIpv6));
        var routedCidrsV4Array = RenderBashArray("HASHI_NETBIRD_ROUTED", routedCidrs.Where(x => !IsIpv6(x)));
        var routedCidrsV6Array = RenderBashArray("HASHI_NETBIRD_ROUTED6", routedCidrs.Where(IsIpv6));
        var trustedIps = string.Join('\n', (host.TrustedPublicIps ?? []).Select(ip =>
            $"ipset add {(IsIpv6(ip) ? "hashi_trusted6" : "hashi_trusted")} {ShellEscape(ip)} -exist"));
        var blockedIps = string.Join('\n', (host.BlockedIps ?? []).Select(ip =>
            $"ipset add {(IsIpv6(ip) ? "hashi_blocked6" : "hashi_blocked")} {ShellEscape(ip)} -exist"));
        var dnatRules = string.Join('\n', (host.PortForwards ?? []).Select(p =>
            IsIpv6(p.TargetHost)
                ? $"ip6tables -t nat -A HASHI_DNAT -p {ShellEscape(p.Protocol.ToLowerInvariant())} --dport {p.PublicPort} -j DNAT --to-destination {ShellEscape($"[{p.TargetHost}]:{p.TargetPort}")}"
                : $"iptables -t nat -A HASHI_DNAT -p {ShellEscape(p.Protocol.ToLowerInvariant())} --dport {p.PublicPort} -j DNAT --to-destination {ShellEscape(p.TargetHost)}:{p.TargetPort}"));
        var fwdRules = string.Join('\n', (host.PortForwards ?? []).Select(p =>
            $"{(IsIpv6(p.TargetHost) ? "ip6tables" : "iptables")} -A HASHI_FWD -p {ShellEscape(p.Protocol.ToLowerInvariant())} -d {ShellEscape(p.TargetHost)} --dport {p.TargetPort} -j ACCEPT"));
        var wan = host.WanInterface is not null
            ? ShellEscape(host.WanInterface)
            : "\"${WAN_IF:-$(ip route show default | awk '{print $5}' | head -1)}\"";
        var publicIp = host.PublicIp is not null
            ? ShellEscape(host.PublicIp)
            : "\"${PUBLIC_IP:-$(ip -4 addr show dev \"$WAN_IF\" | awk '/inet / {print $2}' | cut -d/ -f1 | head -1)}\"";
        var rollbackTimer = host.RollbackTimerSeconds;

        var netbirdChainCreation = host.NetBirdEnabled
            ? """
              iptables -N HASHI_NETBIRD 2>/dev/null || iptables -F HASHI_NETBIRD
              ip6tables -N HASHI_NETBIRD 2>/dev/null || ip6tables -F HASHI_NETBIRD
              """
            : "";

        var netbirdForwardJump = host.NetBirdEnabled
            ? """
              iptables -C HASHI_FWD -j HASHI_NETBIRD 2>/dev/null || iptables -I HASHI_FWD 1 -j HASHI_NETBIRD
              ip6tables -C HASHI_FWD -j HASHI_NETBIRD 2>/dev/null || ip6tables -I HASHI_FWD 1 -j HASHI_NETBIRD
              """
            : "";

        return $$"""
            #!/bin/bash
            # Hashi-managed firewall script for {{ShellEscape(host.Name)}} (spec section 14)
            set -euo pipefail

            ROLLBACK_TIMER={{rollbackTimer}}
            WAN_IF={{wan}}
            PUBLIC_IP={{publicIp}}
            TRAEFIK_IP={{ShellEscape(host.InternalTraefikIp)}}
            NETBIRD_IF={{ShellEscape(host.NetBirdInterface)}}
            ROLLBACK_PID_FILE="/run/hashi-firewall.rollback.pid"

            require_command() {
              if ! command -v "$1" >/dev/null 2>&1; then
                echo "[hashi-firewall] Missing required command: $1" >&2
                exit 2
              fi
            }

            for cmd in iptables ip6tables ipset ip sysctl awk cut head; do
              require_command "$cmd"
            done

            if ! command -v netfilter-persistent >/dev/null 2>&1 && ! command -v systemctl >/dev/null 2>&1 && [[ ! -d /etc/cron.d && ! -w /etc ]]; then
              echo "[hashi-firewall] Missing persistence support: netfilter-persistent, systemctl, or /etc/cron.d" >&2
              exit 2
            fi

            rollback() {
              echo "[hashi-firewall] Rollback timer expired; restoring previous rules if available."
              if [[ -x /opt/hashi/firewall/hashi-firewall.rollback.sh ]]; then
                /opt/hashi/firewall/hashi-firewall.rollback.sh || true
              fi
            }
            ( sleep "$ROLLBACK_TIMER" && rollback ) &
            ROLLBACK_PID=$!
            echo "$ROLLBACK_PID" > "$ROLLBACK_PID_FILE"

            disarm_rollback() {
              if [[ -n "${ROLLBACK_PID:-}" ]]; then
                kill "$ROLLBACK_PID" 2>/dev/null || true
                wait "$ROLLBACK_PID" 2>/dev/null || true
              fi
              rm -f "$ROLLBACK_PID_FILE"
            }

            sysctl -w net.ipv4.ip_forward=1
            sysctl -w net.ipv6.conf.all.forwarding=1

            ipset create hashi_trusted hash:net family inet hashsize 1024 maxelem 65536 -exist
            ipset create hashi_blocked hash:ip family inet hashsize 1024 maxelem 65536 -exist
            ipset create hashi_netbird hash:net family inet hashsize 1024 maxelem 65536 -exist
            ipset create hashi_trusted6 hash:net family inet6 hashsize 1024 maxelem 65536 -exist
            ipset create hashi_blocked6 hash:ip family inet6 hashsize 1024 maxelem 65536 -exist
            ipset create hashi_netbird6 hash:net family inet6 hashsize 1024 maxelem 65536 -exist
            ipset flush hashi_trusted
            ipset flush hashi_blocked
            ipset flush hashi_trusted6
            ipset flush hashi_blocked6

            {{managedSubnetsV4Array}}
            for subnet in "${HASHI_MANAGED_SUBNETS[@]}"; do
              ipset add hashi_trusted "$subnet" -exist
            done
            {{managedSubnetsV6Array}}
            for subnet in "${HASHI_MANAGED_SUBNETS6[@]}"; do
              ipset add hashi_trusted6 "$subnet" -exist
            done
            {{trustedIps}}
            {{blockedIps}}

            iptables -N HASHI_INPUT 2>/dev/null || iptables -F HASHI_INPUT
            iptables -N HASHI_DNAT 2>/dev/null || iptables -F HASHI_DNAT
            iptables -N HASHI_FWD 2>/dev/null || iptables -F HASHI_FWD
            iptables -N HASHI_POSTROUTING 2>/dev/null || iptables -F HASHI_POSTROUTING
            {{netbirdChainCreation}}

            ip6tables -N HASHI_INPUT 2>/dev/null || ip6tables -F HASHI_INPUT
            ip6tables -N HASHI_DNAT 2>/dev/null || ip6tables -F HASHI_DNAT
            ip6tables -N HASHI_FWD 2>/dev/null || ip6tables -F HASHI_FWD
            ip6tables -N HASHI_POSTROUTING 2>/dev/null || ip6tables -F HASHI_POSTROUTING

            iptables -C INPUT -j HASHI_INPUT 2>/dev/null || iptables -I INPUT 1 -j HASHI_INPUT
            iptables -C FORWARD -j HASHI_FWD 2>/dev/null || iptables -I FORWARD 1 -j HASHI_FWD
            iptables -t nat -C PREROUTING -j HASHI_DNAT 2>/dev/null || iptables -t nat -I PREROUTING 1 -j HASHI_DNAT
            iptables -t nat -C POSTROUTING -j HASHI_POSTROUTING 2>/dev/null || iptables -t nat -I POSTROUTING 1 -j HASHI_POSTROUTING
            {{netbirdForwardJump}}

            ip6tables -C INPUT -j HASHI_INPUT 2>/dev/null || ip6tables -I INPUT 1 -j HASHI_INPUT
            ip6tables -C FORWARD -j HASHI_FWD 2>/dev/null || ip6tables -I FORWARD 1 -j HASHI_FWD
            ip6tables -t nat -C PREROUTING -j HASHI_DNAT 2>/dev/null || ip6tables -t nat -I PREROUTING 1 -j HASHI_DNAT
            ip6tables -t nat -C POSTROUTING -j HASHI_POSTROUTING 2>/dev/null || ip6tables -t nat -I POSTROUTING 1 -j HASHI_POSTROUTING

            iptables -A HASHI_INPUT -i lo -j ACCEPT
            iptables -A HASHI_INPUT -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
            iptables -A HASHI_INPUT -m set --match-set hashi_blocked src -j DROP
            iptables -A HASHI_INPUT -m set --match-set hashi_trusted src -j ACCEPT

            ip6tables -A HASHI_INPUT -i lo -j ACCEPT
            ip6tables -A HASHI_INPUT -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
            ip6tables -A HASHI_INPUT -m set --match-set hashi_blocked6 src -j DROP
            ip6tables -A HASHI_INPUT -m set --match-set hashi_trusted6 src -j ACCEPT

            {{RenderNetBirdRules(host, overlayCidrsV4Array, overlayCidrsV6Array, routedCidrsV4Array, routedCidrsV6Array)}}

            iptables -A HASHI_INPUT -j DROP

            ip6tables -A HASHI_INPUT -j DROP

            {{dnatRules}}
            {{fwdRules}}

            for subnet in "${HASHI_MANAGED_SUBNETS[@]}"; do
              iptables -A HASHI_FWD -s "$subnet" -j ACCEPT
              iptables -A HASHI_FWD -d "$subnet" -j ACCEPT
            done
            for subnet in "${HASHI_MANAGED_SUBNETS6[@]}"; do
              ip6tables -A HASHI_FWD -s "$subnet" -j ACCEPT
              ip6tables -A HASHI_FWD -d "$subnet" -j ACCEPT
            done

            for subnet in "${HASHI_MANAGED_SUBNETS[@]}"; do
              iptables -t nat -A HASHI_POSTROUTING -s "$subnet" -o "$WAN_IF" -j MASQUERADE
              iptables -t nat -A HASHI_POSTROUTING -d "$TRAEFIK_IP" -s "$subnet" -j SNAT --to-source "$PUBLIC_IP"
            done
            for subnet in "${HASHI_MANAGED_SUBNETS6[@]}"; do
              ip6tables -t nat -A HASHI_POSTROUTING -s "$subnet" -o "$WAN_IF" -j MASQUERADE
            done

            if command -v netfilter-persistent >/dev/null 2>&1; then
              netfilter-persistent save || true
            fi

            if command -v systemctl >/dev/null 2>&1; then
              cat > /etc/systemd/system/hashi-firewall.service <<'UNIT'
            [Unit]
            Description=Hashi managed firewall rules
            After=network-online.target
            Wants=network-online.target

            [Service]
            Type=oneshot
            RemainAfterExit=yes
            ExecStart=/opt/hashi/firewall/hashi-firewall.sh
            EnvironmentFile=-/opt/hashi/firewall/hashi-firewall.env

            [Install]
            WantedBy=multi-user.target
            UNIT

              systemctl daemon-reload
              systemctl enable hashi-firewall.service || true
            fi

            # Cron fallback for hosts without systemd
            mkdir -p /etc/cron.d
            cat > /etc/cron.d/hashi-firewall <<'CRON'
            */5 * * * * root /opt/hashi/firewall/hashi-firewall.sh >/dev/null 2>&1
            CRON

            iptables -C INPUT -j HASHI_INPUT
            iptables -C FORWARD -j HASHI_FWD
            iptables -t nat -C PREROUTING -j HASHI_DNAT
            iptables -t nat -C POSTROUTING -j HASHI_POSTROUTING
            ip6tables -C INPUT -j HASHI_INPUT
            ip6tables -C FORWARD -j HASHI_FWD
            ip6tables -t nat -C PREROUTING -j HASHI_DNAT
            ip6tables -t nat -C POSTROUTING -j HASHI_POSTROUTING
            disarm_rollback
            echo "[hashi-firewall] Applied for {{ShellEscape(host.Name)}}."
            """;
    }

    public static string RenderEnvFile(FirewallHostDefinition host)
    {
        ValidateHostDefinition(host);
        return $$"""
            HASHI_HOST={{ShellEscape(host.Name)}}
            HASHI_DOMAIN={{ShellEscape(host.Domain)}}
            HASHI_TRAEFIK_IP={{ShellEscape(host.InternalTraefikIp)}}
            HASHI_WAN_IF={{(host.WanInterface is not null ? ShellEscape(host.WanInterface) : "")}}
            HASHI_PUBLIC_IP={{(host.PublicIp is not null ? ShellEscape(host.PublicIp) : "")}}
            HASHI_NETBIRD_IF={{ShellEscape(host.NetBirdInterface)}}
            """;
    }

    private static string RenderNetBirdRules(
        FirewallHostDefinition host,
        string overlayCidrsV4Array,
        string overlayCidrsV6Array,
        string routedCidrsV4Array,
        string routedCidrsV6Array)
    {
        if (!host.NetBirdEnabled)
        {
            return "# NetBird rules disabled\n";
        }

        var lines = new List<string>
        {
            overlayCidrsV4Array,
            "for cidr in \"${HASHI_NETBIRD_OVERLAY[@]}\"; do",
            "  ipset add hashi_netbird \"$cidr\" -exist",
            "done",
            overlayCidrsV6Array,
            "for cidr in \"${HASHI_NETBIRD_OVERLAY6[@]}\"; do",
            "  ipset add hashi_netbird6 \"$cidr\" -exist",
            "done",
            "iptables -A HASHI_INPUT -i \"$NETBIRD_IF\" -m set --match-set hashi_netbird src -j ACCEPT",
            "iptables -A HASHI_NETBIRD -i \"$NETBIRD_IF\" -m set --match-set hashi_netbird src -j ACCEPT",
            "ip6tables -A HASHI_INPUT -i \"$NETBIRD_IF\" -m set --match-set hashi_netbird6 src -j ACCEPT",
            "ip6tables -A HASHI_NETBIRD -i \"$NETBIRD_IF\" -m set --match-set hashi_netbird6 src -j ACCEPT",
        };

        if (host.NetBirdRoutingPeer && (host.NetBirdRoutedCidrs ?? []).Count > 0)
        {
            var overlay = host.NetBirdOverlayCidrs ?? ["100.110.0.0/16"];
            var routed = host.NetBirdRoutedCidrs ?? [];
            if (routed.Any(x => !IsIpv6(x)))
            {
                lines.Add(routedCidrsV4Array);
                lines.Add("for cidr in \"${HASHI_NETBIRD_ROUTED[@]}\"; do");
                lines.Add("  iptables -A HASHI_NETBIRD -i \"$NETBIRD_IF\" -d \"$cidr\" -j ACCEPT");
                if (overlay.Any(x => !IsIpv6(x)))
                {
                    lines.Add("  iptables -A HASHI_POSTROUTING -s \"${HASHI_NETBIRD_OVERLAY[0]}\" -d \"$cidr\" -j MASQUERADE");
                }
                lines.Add("  iptables -A HASHI_NETBIRD -p tcp -m tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu");
                lines.Add("done");
            }
            if (routed.Any(IsIpv6))
            {
                lines.Add(routedCidrsV6Array);
                lines.Add("for cidr in \"${HASHI_NETBIRD_ROUTED6[@]}\"; do");
                lines.Add("  ip6tables -A HASHI_NETBIRD -i \"$NETBIRD_IF\" -d \"$cidr\" -j ACCEPT");
                if (overlay.Any(IsIpv6))
                {
                    lines.Add("  ip6tables -t nat -A HASHI_POSTROUTING -s \"${HASHI_NETBIRD_OVERLAY6[0]}\" -d \"$cidr\" -j MASQUERADE");
                }
                lines.Add("  ip6tables -A HASHI_NETBIRD -p tcp -m tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu");
                lines.Add("done");
            }
        }

        return string.Join('\n', lines);
    }

    private static string RenderBashArray(string name, IEnumerable<string> values)
    {
        var entries = values.Select(value => $"  {ShellEscape(value)}");
        return $"{name}=(\n{string.Join("\n", entries)}\n)";
    }

    private static bool IsIpv6(string value)
        => value.Split('/', 2, StringSplitOptions.TrimEntries)[0].Contains(':', StringComparison.Ordinal);
}
