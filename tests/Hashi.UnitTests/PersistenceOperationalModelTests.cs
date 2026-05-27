using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Microsoft.EntityFrameworkCore;
using Xunit;

namespace Hashi.UnitTests;

public sealed class PersistenceOperationalModelTests
{
    [Fact]
    public async Task Dns_ownership_can_be_queried_independently_from_provider_comments()
    {
        await using var db = CreateDb();
        var connection = new ConnectionEntity { Name = "Hetzner", Type = ConnectionTypeNames.DnsProvider };
        var zone = new DnsZoneEntity { Connection = connection, ProviderZoneId = "zone-1", Name = "example.com" };
        var resource = new ResourceEntity
        {
            Name = "App",
            Slug = "app",
            Domain = "app.example.com",
            Ownership = ResourceOwnershipNames.Managed,
            OwningWorkflow = "resource-sync",
        };
        var record = new DnsRecordEntity
        {
            Zone = zone,
            ProviderRecordId = "provider-1",
            Name = "app.example.com",
            Type = "A",
            Value = "203.0.113.10",
            Ownership = DnsOwnershipNames.Managed,
        };
        db.DnsRecordOwnership.Add(new DnsRecordOwnershipEntity
        {
            Zone = zone,
            DnsRecord = record,
            Resource = resource,
            ProviderRecordId = record.ProviderRecordId,
            Name = record.Name,
            Type = record.Type,
            Value = record.Value,
            Ownership = DnsOwnershipNames.Managed,
            OwnerWorkflow = "resource-sync",
            SyncState = DnsOwnershipSyncStateNames.Applied,
            AppliedContentHash = "hash-1",
        });
        await db.SaveChangesAsync();

        var ownership = await db.DnsRecordOwnership
            .Include(x => x.Resource)
            .SingleAsync(x => x.ZoneId == zone.Id && x.Name == "app.example.com" && x.Type == "A");

        Assert.Equal(resource.Id, ownership.ResourceId);
        Assert.Equal("resource-sync", ownership.OwnerWorkflow);
        Assert.Equal(DnsOwnershipSyncStateNames.Applied, ownership.SyncState);
        Assert.Equal(ResourceOwnershipNames.Managed, ownership.Resource!.Ownership);
    }

    [Fact]
    public async Task Notification_routes_and_deliveries_preserve_provider_history()
    {
        await using var db = CreateDb();
        var provider = new NotificationProviderEntity { Name = "Ops", Type = "telegram" };
        var route = new NotificationRouteEntity
        {
            Provider = provider,
            Name = "Critical monitor alerts",
            EventKind = "monitor",
            Severity = "critical",
        };
        db.NotificationDeliveries.Add(new NotificationDeliveryEntity
        {
            Provider = provider,
            Route = route,
            EventKind = "monitor",
            Subject = "App down",
            Status = NotificationDeliveryStatusNames.Failed,
            AttemptCount = 2,
            ErrorDetails = "rate limited",
        });
        await db.SaveChangesAsync();

        var delivery = await db.NotificationDeliveries
            .Include(x => x.Route)
            .Include(x => x.Provider)
            .SingleAsync();

        Assert.Equal("Critical monitor alerts", delivery.Route!.Name);
        Assert.Equal("telegram", delivery.Provider.Type);
        Assert.Equal("rate limited", delivery.ErrorDetails);
    }

    [Fact]
    public async Task Firewall_desired_and_applied_state_is_queryable_per_host()
    {
        await using var db = CreateDb();
        var connection = new ConnectionEntity { Name = "fw", Type = ConnectionTypeNames.FirewallHost };
        var host = new FirewallHostEntity
        {
            ConnectionId = connection.Id,
            Name = "edge-1",
            Domain = "edge.example.com",
            LinkedTraefikHost = "traefik.internal",
            InternalTraefikIp = "10.0.0.2",
        };
        db.Connections.Add(connection);
        db.FirewallHosts.Add(host);
        db.FirewallSubnets.Add(new FirewallSubnetEntity { FirewallHost = host, Cidr = "10.0.0.0/24" });
        db.FirewallPorts.Add(new FirewallPortEntity
        {
            FirewallHost = host,
            PublicPort = 443,
            TargetPort = 8443,
            Protocol = "tcp",
            TargetHost = "10.0.0.2",
            Confirmed = true,
        });
        db.FirewallAllowedSubjects.Add(new FirewallAllowedSubjectEntity
        {
            FirewallHost = host,
            SubjectKind = FirewallSubjectKindNames.Cidr,
            SubjectValue = "100.110.0.0/16",
            Reason = "NetBird overlay",
        });
        db.FirewallGeneratedScripts.Add(new FirewallGeneratedScriptEntity
        {
            FirewallHost = host,
            DesiredContentHash = "desired",
            AppliedContentHash = "applied",
            DesiredScript = "# desired",
            AppliedScript = "# applied",
            Status = FirewallGeneratedScriptStatusNames.Drifted,
            DiffSummary = "script differs",
        });
        await db.SaveChangesAsync();

        var state = await db.FirewallGeneratedScripts
            .Include(x => x.FirewallHost)
            .SingleAsync(x => x.FirewallHostId == host.Id);

        Assert.Equal(FirewallGeneratedScriptStatusNames.Drifted, state.Status);
        Assert.Equal("script differs", state.DiffSummary);
        Assert.Equal(1, await db.FirewallPorts.CountAsync(x => x.FirewallHostId == host.Id && x.Confirmed));
        Assert.Equal(1, await db.FirewallAllowedSubjects.CountAsync(x => x.FirewallHostId == host.Id));
    }

    [Fact]
    public async Task Background_jobs_expose_required_operational_fields()
    {
        await using var db = CreateDb();
        var started = DateTimeOffset.UtcNow.AddMinutes(-2);
        db.BackgroundJobs.Add(new BackgroundJobEntity
        {
            JobKey = "sync.reconcile",
            DisplayName = "Sync reconcile",
            Status = "failed",
            LastStartedAtUtc = started,
            LastCompletedAtUtc = started.AddSeconds(5),
            NextRunAtUtc = started.AddMinutes(5),
            LastDurationMs = 5000,
            LastDiffSummary = "1 DNS change",
            LastError = "provider unavailable",
        });
        await db.SaveChangesAsync();

        var job = await db.BackgroundJobs.SingleAsync(x => x.JobKey == "sync.reconcile");

        Assert.Equal("failed", job.Status);
        Assert.Equal(5000, job.LastDurationMs);
        Assert.Equal("1 DNS change", job.LastDiffSummary);
        Assert.Equal("provider unavailable", job.LastError);
        Assert.NotNull(job.NextRunAtUtc);
    }

    private static HashiDbContext CreateDb()
    {
        var options = new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        return new HashiDbContext(options);
    }
}
