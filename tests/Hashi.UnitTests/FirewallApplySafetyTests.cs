using System.Text.Json;
using Hashi.Contracts.Api;
using Hashi.Core.Connections;
using Hashi.Core.Firewall;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.UnitTests.Fakes;
using Microsoft.EntityFrameworkCore;
using Xunit;

namespace Hashi.UnitTests;

public sealed class FirewallApplySafetyTests
{
    [Fact]
    public void Render_scopes_public_forwarding_and_netbird_mss_to_hashi_chains()
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
            NetBirdRoutingPeer: true,
            PortForwards: [new FirewallPortForward("tcp", 443, "10.0.0.2", 443)]));

        Assert.DoesNotContain("-d \"$PUBLIC_IP\" -j ACCEPT", script);
        Assert.DoesNotContain("iptables -A FORWARD -p tcp", script);
        Assert.DoesNotContain("iptables -A HASHI_POSTROUTING -o \"$WAN_IF\" -j MASQUERADE", script);
        Assert.Contains("iptables -A HASHI_NETBIRD -p tcp", script);
        Assert.Contains("iptables -C HASHI_FWD -j HASHI_NETBIRD", script);
        Assert.Contains("disarm_rollback", script);
        Assert.Contains("rm -f \"$ROLLBACK_PID_FILE\"", script);
    }

    [Fact]
    public async Task Apply_stops_before_writes_when_preflight_fails()
    {
        await using var db = CreateDb();
        var host = SeedFirewallHost(db);
        await db.SaveChangesAsync();

        var ssh = new FakeSshRemoteExecutor();
        ssh.CommandResults.Enqueue(new RemoteCommandResult(false, string.Empty, "Missing required firewall capabilities: ipset"));
        var service = TestPlatformHelpers.CreateFirewallApply(db, ssh);

        var result = await service.ApplyAsync(BuildRequest(host.Id));

        Assert.False(result.Succeeded);
        Assert.Equal(0, ssh.WriteCount);
        Assert.Contains("Missing required firewall capabilities", result.Message);
        Assert.Contains(ssh.Commands, command => command.Contains("for cmd in iptables ipset ip sysctl", StringComparison.Ordinal));
        Assert.True(await db.SyncDiffs.AnyAsync(x => x.ResourceType == "firewall-script" && x.ResourceKey == "fw1"));
    }

    [Fact]
    public async Task Apply_records_plan_and_verifies_rollback_is_disarmed_after_script_runs()
    {
        await using var db = CreateDb();
        var host = SeedFirewallHost(db);
        db.TraefikEntryPoints.Add(new TraefikEntryPointEntity { Port = 443, Protocol = "tcp", Confirmed = true });
        db.Resources.Add(new ResourceEntity
        {
            Name = "tcp app",
            Slug = "tcp-app",
            Kind = "tcp",
            Enabled = true,
            TargetPort = 443,
            PublicPort = 443,
        });
        await db.SaveChangesAsync();

        var ssh = new FakeSshRemoteExecutor();
        ssh.CommandResults.Enqueue(new RemoteCommandResult(true, "hashi-firewall-preflight-ok", null));
        ssh.CommandResults.Enqueue(new RemoteCommandResult(true, "no", null));
        ssh.CommandResults.Enqueue(new RemoteCommandResult(true, string.Empty, null));
        ssh.CommandResults.Enqueue(new RemoteCommandResult(true, string.Empty, null));
        ssh.CommandResults.Enqueue(new RemoteCommandResult(true, string.Empty, null));
        var service = TestPlatformHelpers.CreateFirewallApply(db, ssh);

        var result = await service.ApplyAsync(BuildRequest(host.Id));

        Assert.True(result.Succeeded);
        Assert.False(result.Skipped);
        Assert.NotNull(result.PlanId);
        Assert.NotNull(result.ScriptHash);
        Assert.Contains("--dport 443", result.Preview);
        Assert.Contains("/run/hashi-firewall.rollback.pid", ssh.Commands[^1]);
        Assert.Contains("iptables -C INPUT -j HASHI_INPUT", ssh.Commands[^1]);
        Assert.Contains("/opt/hashi/firewall/hashi-firewall.sh", ssh.WrittenFiles.Keys);
        Assert.True(await db.SyncRuns.AnyAsync(x => x.Subsystem == "firewall" && x.Status == "succeeded"));
        Assert.True(await db.AuditEvents.AnyAsync(x => x.Action == "script_applied"));
    }

    [Fact]
    public async Task BuildHostDefinition_adds_standard_web_forwards_for_http_resources()
    {
        await using var db = CreateDb();
        var host = SeedFirewallHost(db);
        db.Resources.Add(new ResourceEntity
        {
            Name = "web app",
            Slug = "web-app",
            Kind = "https",
            Enabled = true,
            TargetPort = 8443,
        });
        await db.SaveChangesAsync();

        var service = TestPlatformHelpers.CreateFirewallApply(db);

        var definition = await service.BuildHostDefinitionAsync(host);

        Assert.Contains(definition.PortForwards!, x =>
            x.Protocol == "tcp" && x.PublicPort == 80 && x.TargetHost == host.InternalTraefikIp && x.TargetPort == 80);
        Assert.Contains(definition.PortForwards!, x =>
            x.Protocol == "tcp" && x.PublicPort == 443 && x.TargetHost == host.InternalTraefikIp && x.TargetPort == 443);
    }

    [Fact]
    public async Task BuildHostDefinition_includes_only_active_ip_blocklist_entries()
    {
        await using var db = CreateDb();
        var host = SeedFirewallHost(db);
        db.BlocklistEntries.AddRange(
            new BlocklistEntryEntity
            {
                Type = BlocklistTypeNames.Ip,
                Value = "198.51.100.50",
                ClientIp = "198.51.100.50",
                Reason = "active",
            },
            new BlocklistEntryEntity
            {
                Type = BlocklistTypeNames.Ip,
                Value = "198.51.100.51",
                ClientIp = "198.51.100.51",
                Reason = "expired",
                ExpiresAtUtc = DateTimeOffset.UtcNow.AddMinutes(-1),
            },
            new BlocklistEntryEntity
            {
                Type = BlocklistTypeNames.Asn,
                Value = "AS13335",
                Reason = "asn",
            });
        await db.SaveChangesAsync();

        var service = TestPlatformHelpers.CreateFirewallApply(db);

        var definition = await service.BuildHostDefinitionAsync(host);

        Assert.Contains("198.51.100.50", definition.BlockedIps!);
        Assert.DoesNotContain("198.51.100.51", definition.BlockedIps!);
        Assert.DoesNotContain("AS13335", definition.BlockedIps!);
    }

    [Fact]
    public async Task BuildHostDefinition_adds_web_forwards_for_system_resource()
    {
        await using var db = CreateDb();
        var host = SeedFirewallHost(db);
        var resource = new ResourceEntity
        {
            Name = "Hashi Admin",
            Slug = "hashi-admin",
            Kind = "https",
            Enabled = true,
            IsSystem = true,
            TargetPort = 8080,
        };
        db.Resources.Add(resource);
        db.SystemResources.Add(new SystemResourceEntity
        {
            Resource = resource,
            SystemKey = "hashi-admin",
        });
        await db.SaveChangesAsync();

        var service = TestPlatformHelpers.CreateFirewallApply(db);

        var definition = await service.BuildHostDefinitionAsync(host);

        Assert.Contains(definition.PortForwards!, x => x.Protocol == "tcp" && x.PublicPort == 80 && x.TargetPort == 80);
        Assert.Contains(definition.PortForwards!, x => x.Protocol == "tcp" && x.PublicPort == 443 && x.TargetPort == 443);
    }

    [Fact]
    public async Task BuildHostDefinition_deduplicates_web_and_stream_forwards()
    {
        await using var db = CreateDb();
        var host = SeedFirewallHost(db);
        db.Resources.Add(new ResourceEntity
        {
            Name = "web app",
            Slug = "web-app",
            Kind = "https",
            Enabled = true,
            TargetPort = 8443,
        });
        db.Resources.Add(new ResourceEntity
        {
            Name = "tcp app",
            Slug = "tcp-app",
            Kind = "tcp",
            Enabled = true,
            TargetPort = 443,
            PublicPort = 443,
        });
        await db.SaveChangesAsync();

        var service = TestPlatformHelpers.CreateFirewallApply(db);

        var definition = await service.BuildHostDefinitionAsync(host);

        Assert.Single(definition.PortForwards!, x => x.Protocol == "tcp" && x.PublicPort == 443 && x.TargetPort == 443);
    }

    private static HashiDbContext CreateDb()
    {
        var options = new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        return new HashiDbContext(options);
    }

    private static FirewallHostEntity SeedFirewallHost(HashiDbContext db)
    {
        var host = new FirewallHostEntity
        {
            Name = "fw1",
            Domain = "example.com",
            ManagedSubnetsJson = JsonSerializer.Serialize(new[] { "192.168.1.0/24" }),
            LinkedTraefikHost = "traefik.local",
            InternalTraefikIp = "10.0.0.2",
            PublicIp = "203.0.113.5",
            WanInterface = "eth0",
        };
        db.FirewallHosts.Add(host);
        return host;
    }

    private static FirewallApplyRequest BuildRequest(Guid hostId) => new(
        hostId,
        "203.0.113.5",
        22,
        "root",
        "password",
        "secret",
        null,
        null);
}
