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
            new ResourceDnsTarget("App", "app", "example.com", null, null, null, new PulseDnsTarget(Guid.NewGuid(), "203.0.113.10", "10.0.0.5")),
            hosts);

        Assert.Single(records);
        Assert.Equal(DnsRecordType.Cname, records[0].Type);
        Assert.Contains("on.machine1.example.com", records[0].Value);
    }

    [Fact]
    public void GenerateResourceRecords_uses_cname_when_firewall_host_id_set()
    {
        var hostId = Guid.NewGuid();
        var hosts = new[] { new FirewallHostDnsTarget(hostId, "machine1", "203.0.113.10", null) };
        var records = DnsRecordGenerator.GenerateResourceRecords(
            new ResourceDnsTarget("App", "app", "example.com", null, hostId, null, null),
            hosts);

        Assert.Single(records);
        Assert.Equal(DnsRecordType.Cname, records[0].Type);
        Assert.Contains("on.machine1.example.com", records[0].Value);
    }

    [Fact]
    public void GenerateResourceRecords_uses_cname_when_manual_private_ip_matches_managed_subnet()
    {
        var hosts = new[]
        {
            new FirewallHostDnsTarget(Guid.NewGuid(), "machine1", "203.0.113.10", ManagedSubnets: ["10.0.0.0/24"]),
        };

        var records = DnsRecordGenerator.GenerateResourceRecords(
            new ResourceDnsTarget("App", "app", "example.com", null, null, "10.0.0.25", null),
            hosts);

        Assert.Single(records);
        Assert.Equal(DnsRecordType.Cname, records[0].Type);
        Assert.Equal("on.machine1.example.com", records[0].Value);
    }

    [Theory]
    [InlineData("10.44.7.10")]
    [InlineData(null)]
    public void GenerateResourceRecords_uses_cname_when_target_matches_netbird_routed_cidr(string? manualIp)
    {
        var hosts = new[]
        {
            new FirewallHostDnsTarget(Guid.NewGuid(), "machine1", "203.0.113.10", NetBirdRoutedCidrs: ["10.44.0.0/16"]),
        };
        var pulse = manualIp is null
            ? new PulseDnsTarget(Guid.NewGuid(), "198.51.100.25", "10.44.9.20")
            : null;

        var records = DnsRecordGenerator.GenerateResourceRecords(
            new ResourceDnsTarget("App", "app", "example.com", null, null, manualIp, pulse),
            hosts);

        Assert.Single(records);
        Assert.Equal(DnsRecordType.Cname, records[0].Type);
        Assert.Equal("on.machine1.example.com", records[0].Value);
    }

    [Theory]
    [InlineData("edge.example.com", null)]
    [InlineData(null, "edge.example.com")]
    public void GenerateResourceRecords_uses_cname_when_target_matches_configured_host_fqdn(string? manualHost, string? pulseHostname)
    {
        var hosts = new[]
        {
            new FirewallHostDnsTarget(
                Guid.NewGuid(),
                "machine1",
                "203.0.113.10",
                ConfiguredFqdns: ["edge.example.com"]),
        };
        var pulse = pulseHostname is null
            ? null
            : new PulseDnsTarget(Guid.NewGuid(), "198.51.100.25", "10.10.10.10", pulseHostname);

        var records = DnsRecordGenerator.GenerateResourceRecords(
            new ResourceDnsTarget("App", "app", "example.com", null, null, null, pulse, manualHost),
            hosts);

        Assert.Single(records);
        Assert.Equal(DnsRecordType.Cname, records[0].Type);
        Assert.Equal("on.machine1.example.com", records[0].Value);
    }

    [Fact]
    public void GenerateResourceRecords_uses_a_record_for_unmatched_public_manual_ip()
    {
        var records = DnsRecordGenerator.GenerateResourceRecords(
            new ResourceDnsTarget("App", "app", "example.com", null, null, "198.51.100.55", null),
            []);

        Assert.Single(records);
        Assert.Equal(DnsRecordType.A, records[0].Type);
        Assert.Equal("198.51.100.55", records[0].Value);
    }

    [Fact]
    public void GenerateResourceRecords_does_not_publish_unmatched_private_manual_ip()
    {
        var records = DnsRecordGenerator.GenerateResourceRecords(
            new ResourceDnsTarget("App", "app", "example.com", null, null, "10.0.0.55", null),
            []);

        Assert.Empty(records);
    }

    [Fact]
    public void GenerateResourceRecords_uses_configured_custom_domain()
    {
        var records = DnsRecordGenerator.GenerateResourceRecords(
            new ResourceDnsTarget("App", "app", "example.com", "service.custom.test", null, null, new PulseDnsTarget(Guid.NewGuid(), "203.0.113.20", null)),
            []);

        Assert.Single(records);
        Assert.Equal("service.custom.test", records[0].Name);
        Assert.Equal("203.0.113.20", records[0].Value);
    }

    [Theory]
    [InlineData("example.com", "example.com")]
    [InlineData("@", "example.com")]
    [InlineData(null, "app.example.com")]
    public void ResolveResourceFqdn_supports_root_and_slug_fallback(string? domain, string expected)
    {
        var fqdn = DnsRecordGenerator.ResolveResourceFqdn(
            new ResourceDnsTarget("App", "app", "example.com", domain, null, null, null));

        Assert.Equal(expected, fqdn);
    }

    [Fact]
    public void GenerateHostRecords_skips_hosts_without_public_ip()
    {
        var records = DnsRecordGenerator.GenerateHostRecords(
            new FirewallHostDnsTarget(Guid.NewGuid(), "machine1", null, null),
            "example.com");

        Assert.Empty(records);
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
        Assert.Contains("hashi-firewall.service", script);
        Assert.Contains("systemctl enable hashi-firewall.service", script);
    }

    [Fact]
    public void Render_passes_shellcheck_when_available()
    {
        if (OperatingSystem.IsWindows())
        {
            return;
        }

        var shellcheck = FindShellcheck();
        if (shellcheck is null)
        {
            return;
        }

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
        var path = Path.Combine(Path.GetTempPath(), $"hashi-firewall-{Guid.NewGuid():N}.sh");
        File.WriteAllText(path, script);
        try
        {
            var psi = new System.Diagnostics.ProcessStartInfo(shellcheck)
            {
                RedirectStandardOutput = true,
                RedirectStandardError = true,
            };
            psi.ArgumentList.Add(path);
            using var process = System.Diagnostics.Process.Start(psi)
                ?? throw new InvalidOperationException("Failed to start shellcheck.");
            process.WaitForExit(30_000);
            Assert.True(process.ExitCode == 0, process.StandardOutput.ReadToEnd() + process.StandardError.ReadToEnd());
        }
        finally
        {
            File.Delete(path);
        }
    }

    private static string? FindShellcheck()
    {
        foreach (var candidate in new[] { "/usr/bin/shellcheck", "/bin/shellcheck" })
        {
            if (File.Exists(candidate))
            {
                return candidate;
            }
        }

        var pathEnv = Environment.GetEnvironmentVariable("PATH");
        if (!string.IsNullOrWhiteSpace(pathEnv))
        {
            foreach (var dir in pathEnv.Split(':', StringSplitOptions.RemoveEmptyEntries | StringSplitOptions.TrimEntries))
            {
                var candidate = Path.Combine(dir, "shellcheck");
                if (File.Exists(candidate))
                {
                    return candidate;
                }
            }
        }

        return null;
    }
}
