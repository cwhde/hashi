using Hashi.Core.Dns;
using Hashi.Infrastructure.Dns;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Hashi.Infrastructure.Services;
using Microsoft.EntityFrameworkCore;
using Xunit;

namespace Hashi.UnitTests;

public sealed class DnsRecordServiceTests
{
    [Fact]
    public async Task Create_update_and_delete_manual_record_preserves_user_ownership()
    {
        await using var db = CreateDb();
        var zone = await SeedZoneAsync(db);
        var service = CreateService(db);

        var created = await service.CreateManualAsync(zone.Id, "app.example.com", "A", "203.0.113.10", 300, true);

        Assert.Equal(DnsOwnershipNames.User, created.Ownership);
        Assert.True(created.Enabled);
        Assert.Contains(await db.DnsRecordOwnership.ToListAsync(), x =>
            x.DnsRecordId == created.Id && x.OwnerWorkflow == "manual_dns");

        var updated = await service.UpdateManualAsync(created.Id, zone.Id, "app.example.com", "TXT", "hashi", 600, false);

        Assert.NotNull(updated);
        Assert.Equal("TXT", updated.Type);
        Assert.False(updated.Enabled);
        var desired = await DnsDesiredStateBuilder.BuildAsync(db, zone.Id, zone.DefaultTtl);
        Assert.DoesNotContain(desired, x => x.Name == "app.example.com");

        Assert.True(await service.DeleteManualAsync(created.Id));
        Assert.Empty(await db.DnsRecords.ToListAsync());
        Assert.Empty(await db.DnsRecordOwnership.ToListAsync());
    }

    [Fact]
    public async Task Manual_record_changes_write_subject_based_success_audit_events()
    {
        await using var db = CreateDb();
        var zone = await SeedZoneAsync(db);
        var service = CreateService(db);

        var created = await service.CreateManualAsync(zone.Id, "app.example.com", "A", "203.0.113.10", 300, true);
        await service.UpdateManualAsync(created.Id, zone.Id, "app.example.com", "TXT", "hashi", 600, false);
        await service.DeleteManualAsync(created.Id);

        var events = await db.AuditEvents
            .Where(x => x.Category == "dns" && x.Action.StartsWith("manual_record_"))
            .ToListAsync();

        Assert.Equal(3, events.Count);
        AssertManualDnsAuditEvent(events.Single(x => x.Action == "manual_record_created"), "manual_record_created", created.Id);
        AssertManualDnsAuditEvent(events.Single(x => x.Action == "manual_record_updated"), "manual_record_updated", created.Id);
        AssertManualDnsAuditEvent(events.Single(x => x.Action == "manual_record_deleted"), "manual_record_deleted", created.Id);
        Assert.DoesNotContain(events, x => x.Outcome == "dns_record");
    }

    [Theory]
    [InlineData("NS")]
    [InlineData("SOA")]
    [InlineData("SRV")]
    public async Task CreateManualAsync_rejects_unsupported_record_types(string type)
    {
        await using var db = CreateDb();
        var zone = await SeedZoneAsync(db);
        var service = CreateService(db);

        var ex = await Assert.ThrowsAsync<InvalidOperationException>(() =>
            service.CreateManualAsync(zone.Id, "example.com", type, "value", 300, true));

        Assert.Contains("A, AAAA, CNAME, MX, and TXT", ex.Message);
    }

    [Fact]
    public async Task Generated_records_do_not_replace_user_or_imported_manual_names()
    {
        await using var db = CreateDb();
        var zone = await SeedZoneAsync(db);
        db.AppSettings.Add(new AppSettingsEntity { RootDomain = "example.com" });
        db.DnsRecords.Add(new DnsRecordEntity
        {
            ZoneId = zone.Id,
            Name = "app.example.com",
            Type = "A",
            Value = "198.51.100.10",
            Ownership = DnsOwnershipNames.User,
            Enabled = true,
        });
        db.FirewallHosts.Add(new FirewallHostEntity
        {
            ConnectionId = Guid.NewGuid(),
            Name = "app",
            Domain = "app.example.com",
            PublicIp = "203.0.113.20",
            ManagedSubnetsJson = "[]",
        });
        await db.SaveChangesAsync();

        var desired = await DnsDesiredStateBuilder.BuildAsync(db, zone.Id, zone.DefaultTtl);

        Assert.Contains(desired, x => x.Name == "app.example.com" && x.Value == "198.51.100.10");
        Assert.DoesNotContain(desired, x => x.Name == "app.example.com" && x.Value == "203.0.113.20");
    }

    [Fact]
    public async Task Duplicate_enabled_manual_record_key_is_rejected()
    {
        await using var db = CreateDb();
        var zone = await SeedZoneAsync(db);
        var service = CreateService(db);
        await service.CreateManualAsync(zone.Id, "app.example.com", "A", "203.0.113.10", 300, true);

        var ex = await Assert.ThrowsAsync<InvalidOperationException>(() =>
            service.CreateManualAsync(zone.Id, "app.example.com", "A", "203.0.113.11", 300, true));

        Assert.Contains("same name and type", ex.Message);
    }

    private static HashiDbContext CreateDb()
    {
        var options = new DbContextOptionsBuilder<HashiDbContext>()
            .UseInMemoryDatabase(Guid.NewGuid().ToString())
            .Options;
        return new HashiDbContext(options);
    }

    private static DnsRecordService CreateService(HashiDbContext db)
        => new(db, new AuditService(db));

    private static void AssertManualDnsAuditEvent(AuditEventEntity auditEvent, string action, Guid recordId)
    {
        Assert.Equal("dns", auditEvent.Category);
        Assert.Equal(action, auditEvent.Action);
        Assert.Equal("success", auditEvent.Outcome);
        Assert.Equal("dns_record", auditEvent.SubjectType);
        Assert.Equal(recordId.ToString(), auditEvent.SubjectId);
    }

    private static async Task<DnsZoneEntity> SeedZoneAsync(HashiDbContext db)
    {
        var connection = new ConnectionEntity
        {
            Name = "dns",
            Type = ConnectionTypeNames.DnsProvider,
        };
        var zone = new DnsZoneEntity
        {
            ConnectionId = connection.Id,
            Connection = connection,
            ProviderZoneId = "zone-1",
            Name = "example.com",
            DefaultTtl = 300,
        };
        db.Connections.Add(connection);
        db.DnsZones.Add(zone);
        await db.SaveChangesAsync();
        return zone;
    }
}
