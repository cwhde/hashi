using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Platform;
using Hashi.Infrastructure.Services;
using Microsoft.EntityFrameworkCore;
using Xunit;

namespace Hashi.UnitTests;

public sealed class ConnectionTargetResolverTests
{
    [Fact]
    public async Task Static_targets_resolve_and_build_expected_uris()
    {
        await using var db = CreateDb();
        var resolver = CreateResolver(db);
        var hostTarget = new ConnectionTargetEntity
        {
            OwnerType = "test",
            OwnerId = Guid.NewGuid(),
            TargetMode = ConnectionTargetModeNames.StaticHost,
            StaticHost = "adguard.home",
            Scheme = "https",
            Port = 30443,
            PathPrefix = "/dns",
        };
        db.ConnectionTargets.Add(hostTarget);
        await db.SaveChangesAsync();

        var resolved = await resolver.ResolveAsync(hostTarget);

        Assert.Equal(ConnectionTargetStatusNames.Resolved, resolved.Status);
        Assert.Equal("https://adguard.home:30443/dns", resolved.BaseUri.ToString().TrimEnd('/'));
        Assert.Equal("adguard.home", hostTarget.ResolvedIpSnapshot);
    }

    [Fact]
    public async Task Pulse_public_and_selected_modes_resolve_from_agent()
    {
        await using var db = CreateDb();
        var agentId = SeedAgent(db);
        var resolver = CreateResolver(db);
        var selected = Target(agentId, PulseTargetIpModeNames.Selected);
        var publicTarget = Target(agentId, PulseTargetIpModeNames.Public);
        db.ConnectionTargets.AddRange(selected, publicTarget);
        await db.SaveChangesAsync();

        var selectedResult = await resolver.ResolveAsync(selected);
        var publicResult = await resolver.ResolveAsync(publicTarget);

        Assert.Equal("10.0.0.5", selectedResult.ResolvedIp);
        Assert.Equal("203.0.113.10", publicResult.ResolvedIp);
    }

    [Theory]
    [InlineData("address=10.0.0.6", "10.0.0.6")]
    [InlineData("cidr=10.0.0.0/24", "10.0.0.5")]
    [InlineData("interface=eth0", "10.0.0.5")]
    [InlineData("first_ipv4", "10.0.0.5")]
    public async Task Pulse_private_candidate_selector_resolves_by_address_cidr_interface_and_legacy_selector(
        string selector,
        string expected)
    {
        await using var db = CreateDb();
        var agentId = SeedAgent(db);
        var resolver = CreateResolver(db);
        var target = Target(agentId, PulseTargetIpModeNames.PrivateCandidate, selector);
        db.ConnectionTargets.Add(target);
        await db.SaveChangesAsync();

        var resolved = await resolver.ResolveAsync(target);

        Assert.Equal(ConnectionTargetStatusNames.Resolved, resolved.Status);
        Assert.Equal(expected, resolved.ResolvedIp);
    }

    [Fact]
    public async Task Stale_agent_resolves_last_known_ip_with_stale_status()
    {
        await using var db = CreateDb();
        var agentId = SeedAgent(db, DateTimeOffset.UtcNow.AddMinutes(-20));
        var resolver = CreateResolver(db);
        var target = Target(agentId, PulseTargetIpModeNames.Selected);
        db.ConnectionTargets.Add(target);
        await db.SaveChangesAsync();

        var resolved = await resolver.ResolveAsync(target);

        Assert.Equal(ConnectionTargetStatusNames.Stale, resolved.Status);
        Assert.True(resolved.IsStale);
        Assert.Equal("10.0.0.5", resolved.ResolvedIp);
        Assert.Contains("stale", resolved.Error, StringComparison.OrdinalIgnoreCase);
    }

    [Fact]
    public async Task Missing_agent_fails_without_resolved_snapshot()
    {
        await using var db = CreateDb();
        var resolver = CreateResolver(db);
        var target = Target(Guid.NewGuid(), PulseTargetIpModeNames.Selected);
        db.ConnectionTargets.Add(target);
        await db.SaveChangesAsync();

        var resolved = await resolver.ResolveAsync(target);

        Assert.Equal(ConnectionTargetStatusNames.Failed, resolved.Status);
        Assert.Null(resolved.ResolvedIp);
        Assert.Contains("missing", resolved.Error, StringComparison.OrdinalIgnoreCase);
    }

    [Fact]
    public async Task Pulse_target_change_records_audit_and_pending_sync_run()
    {
        await using var db = CreateDb();
        var agentId = SeedAgent(db);
        var target = Target(agentId, PulseTargetIpModeNames.Selected);
        target.OwnerType = ConnectionTargetOwnerTypeNames.AdGuardConnection;
        target.ResolvedIpSnapshot = "10.0.0.4";
        db.ConnectionTargets.Add(target);
        await db.SaveChangesAsync();
        var resolver = CreateResolver(db);

        await resolver.RefreshTargetsForPulseAgentAsync(agentId);

        Assert.Contains(await db.AuditEvents.ToListAsync(), x =>
            x.Category == "connection_target" && x.Action == "agent_bound_target_changed");
        Assert.Contains(await db.SyncRuns.ToListAsync(), x =>
            x.Subsystem == "adguard-pulse-target" && x.Status == SyncRunStatusNames.Pending);
    }

    private static ConnectionTargetEntity Target(
        Guid agentId,
        string mode,
        string selector = PulsePrivateCandidateSelectorNames.Selected)
        => new()
        {
            OwnerType = "test",
            OwnerId = Guid.NewGuid(),
            TargetMode = ConnectionTargetModeNames.PulseAgent,
            PulseAgentId = agentId,
            PulseIpMode = mode,
            PrivateCandidateSelector = selector,
            Scheme = "http",
            Port = 3000,
        };

    private static Guid SeedAgent(HashiDbContext db, DateTimeOffset? lastSeen = null)
    {
        var agentId = Guid.NewGuid();
        db.PulseAgents.Add(new PulseAgentEntity
        {
            Id = agentId,
            Name = "edge",
            TokenHash = "hash",
            Status = "online",
            HeartbeatIntervalSeconds = 60,
            LastSeenAtUtc = lastSeen ?? DateTimeOffset.UtcNow,
            LastPublicIp = "203.0.113.10",
            LastPrivateIp = "10.0.0.5",
            LastSelectedIp = "10.0.0.5",
            LastSelectedInterface = "eth0",
            LastPrivateIpv4CandidatesJson = """["10.0.0.5","10.0.0.6"]""",
            LastPrivateIpv6CandidatesJson = """["fd00::5"]""",
        });
        return agentId;
    }

    private static ConnectionTargetResolver CreateResolver(HashiDbContext db)
        => new(db, new AuditService(db));

    private static HashiDbContext CreateDb()
        => new(new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options);
}
