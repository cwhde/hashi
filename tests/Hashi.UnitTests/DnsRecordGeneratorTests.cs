using Hashi.Core.Dns;
using Xunit;

namespace Hashi.UnitTests;

public sealed class DnsRecordGeneratorTests
{
    [Fact]
    public void GenerateHostRecords_creates_via_and_on_cnames()
    {
        var records = DnsRecordGenerator.GenerateHostRecords(
            new FirewallHostDnsTarget(Guid.NewGuid(), "machine1", "203.0.113.10", null),
            "example.com");

        Assert.Equal(3, records.Count);
        Assert.Contains(records, r => r.Name == "machine1.example.com" && r.Type == DnsRecordType.A);
        Assert.Contains(records, r => r.Name == "via.machine1.example.com" && r.Type == DnsRecordType.Cname);
        Assert.Contains(records, r => r.Name == "on.machine1.example.com" && r.Type == DnsRecordType.Cname);
    }

    [Fact]
    public void GenerateResourceRecords_uses_cname_when_pulse_ip_matches_host()
    {
        var hosts = new[] { new FirewallHostDnsTarget(Guid.NewGuid(), "machine1", "10.0.0.5", null) };
        var records = DnsRecordGenerator.GenerateResourceRecords(
            new ResourceDnsTarget("App", "app", "example.com", null, null, new PulseDnsTarget(Guid.NewGuid(), "203.0.113.10", "10.0.0.5")),
            hosts);

        Assert.Single(records);
        Assert.Equal(DnsRecordType.Cname, records[0].Type);
        Assert.Contains("on.machine1.example.com", records[0].Value);
    }
}

public sealed class FirewallScriptRendererTests
{
    [Fact]
    public void Render_includes_dnat_netbird_and_cron_stub()
    {
        var script = Hashi.Core.Firewall.FirewallScriptRenderer.Render(new Hashi.Core.Firewall.FirewallHostDefinition(
            Guid.NewGuid(),
            "fw1",
            "example.com",
            ["192.168.1.0/24"],
            "traefik.local",
            "10.0.0.2",
            "203.0.113.5",
            PortForwards: [new Hashi.Core.Firewall.FirewallPortForward("tcp", 443, "10.0.0.2", 443)],
            BlockedIps: ["198.51.100.9"]));
        Assert.Contains("hashi_trusted", script);
        Assert.Contains("HASHI_DNAT", script);
        Assert.Contains("HASHI_NETBIRD", script);
        Assert.Contains("hashi_blocked", script);
        Assert.Contains("--dport 443", script);
        Assert.Contains("rollback", script, StringComparison.OrdinalIgnoreCase);
        Assert.Contains("/etc/cron.d/hashi-firewall", script);
    }
}
