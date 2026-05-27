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
    public static string Render(FirewallHostDefinition host)
    {
        var managedSubnetsArray = RenderBashArray("HASHI_MANAGED_SUBNETS", host.ManagedSubnets);
        var overlayCidrs = host.NetBirdOverlayCidrs ?? ["100.110.0.0/16"];
        var routedCidrs = host.NetBirdRoutedCidrs ?? [];
        var overlayCidrsArray = RenderBashArray("HASHI_NETBIRD_OVERLAY", overlayCidrs);
        var routedCidrsArray = RenderBashArray("HASHI_NETBIRD_ROUTED", routedCidrs);
        var trustedIps = string.Join('\n', (host.TrustedPublicIps ?? []).Select(ip => $"ipset add hashi_trusted {ip} -exist"));
        var blockedIps = string.Join('\n', (host.BlockedIps ?? []).Select(ip => $"ipset add hashi_blocked {ip} -exist"));
        var dnatRules = string.Join('\n', (host.PortForwards ?? []).Select(p =>
            $"iptables -t nat -A HASHI_DNAT -p {p.Protocol.ToLowerInvariant()} --dport {p.PublicPort} -j DNAT --to-destination {p.TargetHost}:{p.TargetPort}"));
        var fwdRules = string.Join('\n', (host.PortForwards ?? []).Select(p =>
            $"iptables -A HASHI_FWD -p {p.Protocol.ToLowerInvariant()} -d {p.TargetHost} --dport {p.TargetPort} -j ACCEPT"));
        var wan = host.WanInterface ?? "${WAN_IF:-$(ip route show default | awk '{print $5}' | head -1)}";
        var publicIp = host.PublicIp ?? "${PUBLIC_IP:-$(ip -4 addr show dev \"$WAN_IF\" | awk '/inet / {print $2}' | cut -d/ -f1 | head -1)}";
        var rollbackTimer = host.RollbackTimerSeconds;

        return $$"""
            #!/bin/bash
            # Hashi-managed firewall script for {{host.Name}} (spec section 14)
            set -euo pipefail

            ROLLBACK_TIMER={{rollbackTimer}}
            WAN_IF="{{wan}}"
            PUBLIC_IP="{{publicIp}}"
            TRAEFIK_IP="{{host.InternalTraefikIp}}"
            NETBIRD_IF="{{host.NetBirdInterface}}"
            ROLLBACK_PID_FILE="/run/hashi-firewall.rollback.pid"

            require_command() {
              if ! command -v "$1" >/dev/null 2>&1; then
                echo "[hashi-firewall] Missing required command: $1" >&2
                exit 2
              fi
            }

            for cmd in iptables ipset ip sysctl awk cut head; do
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

            ipset create hashi_trusted hash:net family inet hashsize 1024 maxelem 65536 -exist
            ipset create hashi_blocked hash:ip family inet hashsize 1024 maxelem 65536 -exist
            ipset create hashi_netbird hash:net family inet hashsize 1024 maxelem 65536 -exist
            ipset flush hashi_trusted
            ipset flush hashi_blocked

            {{managedSubnetsArray}}
            for subnet in "${HASHI_MANAGED_SUBNETS[@]}"; do
              ipset add hashi_trusted "$subnet" -exist
            done
            {{trustedIps}}
            {{blockedIps}}

            iptables -N HASHI_INPUT 2>/dev/null || iptables -F HASHI_INPUT
            iptables -N HASHI_DNAT 2>/dev/null || iptables -F HASHI_DNAT
            iptables -N HASHI_FWD 2>/dev/null || iptables -F HASHI_FWD
            iptables -N HASHI_POSTROUTING 2>/dev/null || iptables -F HASHI_POSTROUTING
            iptables -N HASHI_NETBIRD 2>/dev/null || iptables -F HASHI_NETBIRD

            iptables -C INPUT -j HASHI_INPUT 2>/dev/null || iptables -I INPUT 1 -j HASHI_INPUT
            iptables -C FORWARD -j HASHI_FWD 2>/dev/null || iptables -I FORWARD 1 -j HASHI_FWD
            iptables -t nat -C PREROUTING -j HASHI_DNAT 2>/dev/null || iptables -t nat -I PREROUTING 1 -j HASHI_DNAT
            iptables -t nat -C POSTROUTING -j HASHI_POSTROUTING 2>/dev/null || iptables -t nat -I POSTROUTING 1 -j HASHI_POSTROUTING
            iptables -C HASHI_FWD -j HASHI_NETBIRD 2>/dev/null || iptables -I HASHI_FWD 1 -j HASHI_NETBIRD

            iptables -A HASHI_INPUT -i lo -j ACCEPT
            iptables -A HASHI_INPUT -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
            iptables -A HASHI_INPUT -m set --match-set hashi_blocked src -j DROP
            iptables -A HASHI_INPUT -m set --match-set hashi_trusted src -j ACCEPT

            {{RenderNetBirdRules(host, overlayCidrsArray, routedCidrsArray)}}

            iptables -A HASHI_INPUT -j DROP

            {{dnatRules}}
            {{fwdRules}}

            for subnet in "${HASHI_MANAGED_SUBNETS[@]}"; do
              iptables -A HASHI_FWD -s "$subnet" -j ACCEPT
              iptables -A HASHI_FWD -d "$subnet" -j ACCEPT
            done

            for subnet in "${HASHI_MANAGED_SUBNETS[@]}"; do
              iptables -t nat -A HASHI_POSTROUTING -s "$subnet" -o "$WAN_IF" -j MASQUERADE
              iptables -t nat -A HASHI_POSTROUTING -d "$TRAEFIK_IP" -s "$subnet" -j SNAT --to-source "$PUBLIC_IP"
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
            disarm_rollback
            echo "[hashi-firewall] Applied for {{host.Name}}."
            """;
    }

    public static string RenderEnvFile(FirewallHostDefinition host) => $$"""
        HASHI_HOST={{host.Name}}
        HASHI_DOMAIN={{host.Domain}}
        HASHI_TRAEFIK_IP={{host.InternalTraefikIp}}
        HASHI_WAN_IF={{host.WanInterface ?? ""}}
        HASHI_PUBLIC_IP={{host.PublicIp ?? ""}}
        HASHI_NETBIRD_IF={{host.NetBirdInterface}}
        """;

    private static string RenderNetBirdRules(
        FirewallHostDefinition host,
        string overlayCidrsArray,
        string routedCidrsArray)
    {
        if (!host.NetBirdEnabled)
        {
            return "# NetBird rules disabled\n";
        }

        var lines = new List<string>
        {
            overlayCidrsArray,
            "for cidr in \"${HASHI_NETBIRD_OVERLAY[@]}\"; do",
            "  ipset add hashi_netbird \"$cidr\" -exist",
            "done",
            "iptables -A HASHI_INPUT -i \"$NETBIRD_IF\" -m set --match-set hashi_netbird src -j ACCEPT",
            "iptables -A HASHI_NETBIRD -i \"$NETBIRD_IF\" -m set --match-set hashi_netbird src -j ACCEPT",
        };

        if (host.NetBirdRoutingPeer && (host.NetBirdRoutedCidrs ?? []).Count > 0)
        {
            lines.Add(routedCidrsArray);
            lines.Add("for cidr in \"${HASHI_NETBIRD_ROUTED[@]}\"; do");
            lines.Add("  iptables -A HASHI_NETBIRD -i \"$NETBIRD_IF\" -d \"$cidr\" -j ACCEPT");
            lines.Add("  iptables -A HASHI_POSTROUTING -s \"${HASHI_NETBIRD_OVERLAY[0]}\" -d \"$cidr\" -j MASQUERADE");
            lines.Add("  iptables -A HASHI_NETBIRD -p tcp -m tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu");
            lines.Add("done");
        }

        return string.Join('\n', lines);
    }

    private static string RenderBashArray(string name, IEnumerable<string> values)
    {
        var entries = values.Select(value => $"  \"{value.Replace("\"", "\\\"", StringComparison.Ordinal)}\"");
        return $"{name}=(\n{string.Join("\n", entries)}\n)";
    }
}
