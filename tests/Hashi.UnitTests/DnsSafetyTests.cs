using Hashi.Core.Dns;
using Xunit;

namespace Hashi.UnitTests;

public sealed class DnsSafetyRulesTests
{
    [Theory]
    [InlineData(DnsRecordType.Ns)]
    [InlineData(DnsRecordType.Soa)]
    public void Protected_types_cannot_be_deleted(DnsRecordType type)
    {
        var record = new DnsRecordSnapshot("1", "example.com", type, "value", 3600, false);
        var change = new DnsPlanChange(DnsChangeKind.Delete, record.Name, record.Type, record.Value, null, record.Ttl, "delete");
        var guarded = DnsSafetyRules.GuardChange(change);
        Assert.NotNull(guarded);
        Assert.Equal(DnsChangeKind.NoOp, guarded!.Kind);
        Assert.Contains("NS/SOA", guarded.RiskReason);
    }

    [Theory]
    [InlineData(true, DnsRecordType.A, true)]
    [InlineData(true, DnsRecordType.Ns, false)]
    [InlineData(true, DnsRecordType.Soa, false)]
    [InlineData(false, DnsRecordType.A, false)]
    public void CanDelete_only_allows_managed_non_protected_records(
        bool managedByHashi,
        DnsRecordType type,
        bool expected)
    {
        var record = new DnsRecordSnapshot("1", "example.com", type, "value", 3600, managedByHashi);
        Assert.Equal(expected, DnsSafetyRules.CanDelete(record));
    }

    [Fact]
    public void Planner_skips_deleting_protected_records()
    {
        IReadOnlyList<DnsRecordSnapshot> current =
        [
            new DnsRecordSnapshot("1", "@", DnsRecordType.Ns, "ns1.example.com", 3600, true),
            new DnsRecordSnapshot("2", "app", DnsRecordType.A, "1.2.3.4", 3600, true),
        ];
        IReadOnlyList<DnsRecordSnapshot> desired = [];
        var plan = DnsPlanner.BuildPlan(current, desired);
        Assert.DoesNotContain(plan, x => x.Type == DnsRecordType.Ns && x.Kind == DnsChangeKind.Delete);
    }

    [Theory]
    [InlineData(DnsRecordType.Ns)]
    [InlineData(DnsRecordType.Soa)]
    public void Protected_types_cannot_be_updated(DnsRecordType type)
    {
        var record = new DnsRecordSnapshot("1", "@", type, "old", 3600, true);
        var change = new DnsPlanChange(DnsChangeKind.Update, record.Name, record.Type, record.Value, "new", record.Ttl, "update");
        var guarded = DnsSafetyRules.GuardChange(change);
        Assert.NotNull(guarded);
        Assert.Equal(DnsChangeKind.NoOp, guarded!.Kind);
    }

    [Fact]
    public void Planner_creates_missing_managed_records()
    {
        IReadOnlyList<DnsRecordSnapshot> desired =
        [
            new DnsRecordSnapshot("new", "app", DnsRecordType.A, "1.2.3.4", 3600, true),
        ];
        var plan = DnsPlanner.BuildPlan(Array.Empty<DnsRecordSnapshot>(), desired);
        Assert.Contains(plan, x => x.Kind == DnsChangeKind.Create && x.Name == "app");
    }

    [Fact]
    public void Planner_blocks_update_when_desired_collides_with_unowned_provider_record()
    {
        IReadOnlyList<DnsRecordSnapshot> current =
        [
            new DnsRecordSnapshot("provider-app", "app", DnsRecordType.A, "1.2.3.4", 3600, false),
        ];
        IReadOnlyList<DnsRecordSnapshot> desired =
        [
            new DnsRecordSnapshot(string.Empty, "app", DnsRecordType.A, "203.0.113.10", 3600, true),
        ];

        var plan = DnsPlanner.BuildPlan(current, desired);

        var change = Assert.Single(plan);
        Assert.Equal(DnsChangeKind.NoOp, change.Kind);
        Assert.Equal("provider-app", change.ProviderRecordId);
        Assert.Contains("not owned", change.RiskReason);
    }

    [Theory]
    [InlineData(DnsRecordType.Ns)]
    [InlineData(DnsRecordType.Soa)]
    public void Planner_blocks_protected_record_updates(DnsRecordType type)
    {
        IReadOnlyList<DnsRecordSnapshot> current =
        [
            new DnsRecordSnapshot("protected", "@", type, "old", 3600, true),
        ];
        IReadOnlyList<DnsRecordSnapshot> desired =
        [
            new DnsRecordSnapshot("protected", "@", type, "new", 3600, true),
        ];

        var plan = DnsPlanner.BuildPlan(current, desired);

        var change = Assert.Single(plan);
        Assert.Equal(DnsChangeKind.NoOp, change.Kind);
        Assert.Contains("NS/SOA", change.RiskReason);
    }
}
