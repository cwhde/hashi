using Hashi.Core.Firewall;
using Xunit;

namespace Hashi.UnitTests;

public sealed class NetBirdPreservationTests
{
    [Fact]
    public void Render_preserves_netbird_chains_in_generated_script()
    {
        var script = FirewallScriptRenderer.Render(new FirewallHostDefinition(
            Guid.NewGuid(),
            "fw1",
            "example.com",
            ["192.168.1.0/24"],
            "traefik.local",
            "10.0.0.2",
            "203.0.113.5",
            NetBirdRoutedCidrs: ["10.44.0.0/16"],
            NetBirdRoutingPeer: true));

        Assert.Contains("iptables -N HASHI_NETBIRD", script);
        Assert.Contains("iptables -A HASHI_NETBIRD", script);
        Assert.Contains("iptables -C HASHI_FWD -j HASHI_NETBIRD", script);
    }

    [Fact]
    public void Render_includes_netbird_interface_accept_rules()
    {
        var script = FirewallScriptRenderer.Render(new FirewallHostDefinition(
            Guid.NewGuid(),
            "fw1",
            "example.com",
            ["192.168.1.0/24"],
            "traefik.local",
            "10.0.0.2",
            "203.0.113.5"));

        Assert.Contains("iptables -A HASHI_INPUT -i \"$NETBIRD_IF\"", script);
        Assert.Contains("iptables -A HASHI_NETBIRD -i \"$NETBIRD_IF\"", script);
    }

    [Fact]
    public void Render_does_not_flush_netbird_chains_when_disabled()
    {
        var script = FirewallScriptRenderer.Render(new FirewallHostDefinition(
            Guid.NewGuid(),
            "fw1",
            "example.com",
            ["192.168.1.0/24"],
            "traefik.local",
            "10.0.0.2",
            "203.0.113.5",
            NetBirdEnabled: false));

        Assert.DoesNotContain("iptables -N HASHI_NETBIRD", script);
        Assert.Contains("# NetBird rules disabled", script);
    }

    [Fact]
    public void Render_includes_routed_cidrs_for_routing_peer()
    {
        var script = FirewallScriptRenderer.Render(new FirewallHostDefinition(
            Guid.NewGuid(),
            "fw1",
            "example.com",
            ["192.168.1.0/24"],
            "traefik.local",
            "10.0.0.2",
            "203.0.113.5",
            NetBirdRoutedCidrs: ["10.44.0.0/16", "10.45.0.0/16"],
            NetBirdRoutingPeer: true));

        Assert.Contains("HASHI_NETBIRD_ROUTED", script);
        Assert.Contains("10.44.0.0/16", script);
        Assert.Contains("10.45.0.0/16", script);
        Assert.Contains("iptables -A HASHI_NETBIRD -i \"$NETBIRD_IF\" -d \"$cidr\" -j ACCEPT", script);
    }

    [Fact]
    public void Render_netbird_overlay_cidrs_default_to_100_110_0_0_16()
    {
        var script = FirewallScriptRenderer.Render(new FirewallHostDefinition(
            Guid.NewGuid(),
            "fw1",
            "example.com",
            ["192.168.1.0/24"],
            "traefik.local",
            "10.0.0.2",
            "203.0.113.5"));

        Assert.Contains("100.110.0.0/16", script);
    }

    [Fact]
    public void Render_custom_netbird_overlay_cidrs_override_defaults()
    {
        var script = FirewallScriptRenderer.Render(new FirewallHostDefinition(
            Guid.NewGuid(),
            "fw1",
            "example.com",
            ["192.168.1.0/24"],
            "traefik.local",
            "10.0.0.2",
            "203.0.113.5",
            NetBirdOverlayCidrs: ["10.99.0.0/16"]));

        Assert.Contains("10.99.0.0/16", script);
        Assert.DoesNotContain("100.110.0.0/16", script);
    }

    [Fact]
    public void Render_netbird_chain_is_not_flushed_by_hashi_script()
    {
        var script = FirewallScriptRenderer.Render(new FirewallHostDefinition(
            Guid.NewGuid(),
            "fw1",
            "example.com",
            ["192.168.1.0/24"],
            "traefik.local",
            "10.0.0.2",
            "203.0.113.5"));

        Assert.Contains("iptables -N HASHI_NETBIRD 2>/dev/null || iptables -F HASHI_NETBIRD", script);
    }

    [Fact]
    public void Render_does_not_include_public_ip_in_forward_rules_when_not_specified()
    {
        var script = FirewallScriptRenderer.Render(new FirewallHostDefinition(
            Guid.NewGuid(),
            "fw1",
            "example.com",
            ["192.168.1.0/24"],
            "traefik.local",
            "10.0.0.2"));

        Assert.DoesNotContain("-d \"$PUBLIC_IP\" -j ACCEPT", script);
    }

    [Fact]
    public void Render_env_file_includes_netbird_interface()
    {
        var env = FirewallScriptRenderer.RenderEnvFile(new FirewallHostDefinition(
            Guid.NewGuid(),
            "fw1",
            "example.com",
            ["192.168.1.0/24"],
            "traefik.local",
            "10.0.0.2",
            "203.0.113.5",
            NetBirdInterface: "netbird0"));

        Assert.Contains("HASHI_NETBIRD_IF='netbird0'", env);
    }

    [Fact]
    public void Render_disarm_rollback_removes_pid_file()
    {
        var script = FirewallScriptRenderer.Render(new FirewallHostDefinition(
            Guid.NewGuid(),
            "fw1",
            "example.com",
            ["192.168.1.0/24"],
            "traefik.local",
            "10.0.0.2",
            "203.0.113.5"));

        Assert.Contains("disarm_rollback", script);
        Assert.Contains("rm -f \"$ROLLBACK_PID_FILE\"", script);
    }
}
