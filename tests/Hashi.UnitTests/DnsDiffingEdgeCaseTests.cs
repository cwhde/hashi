using Hashi.Core.Dns;
using Xunit;

namespace Hashi.UnitTests;

public sealed class DnsDiffingEdgeCaseTests
{
    [Fact]
    public void Planner_does_not_delete_unowned_records()
    {
        IReadOnlyList<DnsRecordSnapshot> current =
        [
            new DnsRecordSnapshot("external", "app", DnsRecordType.A, "1.2.3.4", 3600, false),
        ];
        IReadOnlyList<DnsRecordSnapshot> desired = [];

        var plan = DnsPlanner.BuildPlan(current, desired);

        Assert.Empty(plan);
    }

    [Fact]
    public void Planner_does_not_update_unowned_records()
    {
        IReadOnlyList<DnsRecordSnapshot> current =
        [
            new DnsRecordSnapshot("external", "app", DnsRecordType.A, "1.2.3.4", 3600, false),
        ];
        IReadOnlyList<DnsRecordSnapshot> desired =
        [
            new DnsRecordSnapshot("external", "app", DnsRecordType.A, "5.6.7.8", 3600, false),
        ];

        var plan = DnsPlanner.BuildPlan(current, desired);

        var change = Assert.Single(plan);
        Assert.Equal(DnsChangeKind.NoOp, change.Kind);
        Assert.Contains("not owned", change.RiskReason);
    }

    [Theory]
    [InlineData(DnsRecordType.Ns)]
    [InlineData(DnsRecordType.Soa)]
    public void Planner_blocks_deletion_of_protected_record_types(DnsRecordType type)
    {
        IReadOnlyList<DnsRecordSnapshot> current =
        [
            new DnsRecordSnapshot("1", "@", type, "ns1.example.com", 3600, true),
        ];
        IReadOnlyList<DnsRecordSnapshot> desired = [];

        var plan = DnsPlanner.BuildPlan(current, desired);

        var change = Assert.Single(plan);
        Assert.Equal(DnsChangeKind.NoOp, change.Kind);
        Assert.Contains("NS/SOA", change.RiskReason);
    }

    [Theory]
    [InlineData(DnsRecordType.Ns)]
    [InlineData(DnsRecordType.Soa)]
    public void Planner_blocks_modification_of_protected_record_types(DnsRecordType type)
    {
        IReadOnlyList<DnsRecordSnapshot> current =
        [
            new DnsRecordSnapshot("1", "@", type, "old", 3600, true),
        ];
        IReadOnlyList<DnsRecordSnapshot> desired =
        [
            new DnsRecordSnapshot("1", "@", type, "new", 3600, true),
        ];

        var plan = DnsPlanner.BuildPlan(current, desired);

        var change = Assert.Single(plan);
        Assert.Equal(DnsChangeKind.NoOp, change.Kind);
        Assert.Contains("NS/SOA", change.RiskReason);
    }

    [Fact]
    public void Planner_handles_empty_current_and_desired()
    {
        var plan = DnsPlanner.BuildPlan([], []);

        Assert.Empty(plan);
    }

    [Fact]
    public void Planner_handles_duplicate_desired_records()
    {
        IReadOnlyList<DnsRecordSnapshot> desired =
        [
            new DnsRecordSnapshot("1", "app", DnsRecordType.A, "1.2.3.4", 3600, true),
            new DnsRecordSnapshot("2", "app", DnsRecordType.A, "5.6.7.8", 3600, true),
        ];

        var plan = DnsPlanner.BuildPlan([], desired);

        var changes = plan.Where(x => x.Kind == DnsChangeKind.NoOp && x.RiskReason.Contains("conflict")).ToList();
        Assert.Single(changes);
    }

    [Fact]
    public void Planner_handles_multi_value_records_with_same_name()
    {
        IReadOnlyList<DnsRecordSnapshot> current =
        [
            new DnsRecordSnapshot("1", "example.com", DnsRecordType.Mx, "10 mail1.example.com", 3600, true),
            new DnsRecordSnapshot("2", "example.com", DnsRecordType.Mx, "20 mail2.example.com", 3600, true),
        ];
        IReadOnlyList<DnsRecordSnapshot> desired =
        [
            new DnsRecordSnapshot("1", "example.com", DnsRecordType.Mx, "10 mail1.example.com", 3600, true),
            new DnsRecordSnapshot("2", "example.com", DnsRecordType.Mx, "20 mail2.example.com", 3600, true),
        ];

        var plan = DnsPlanner.BuildPlan(current, desired);

        Assert.All(plan, x => Assert.Equal(DnsChangeKind.NoOp, x.Kind));
    }

    [Fact]
    public void Planner_creates_multi_value_records()
    {
        IReadOnlyList<DnsRecordSnapshot> desired =
        [
            new DnsRecordSnapshot("", "example.com", DnsRecordType.Mx, "10 mail1.example.com", 3600, true),
            new DnsRecordSnapshot("", "example.com", DnsRecordType.Mx, "20 mail2.example.com", 3600, true),
        ];

        var plan = DnsPlanner.BuildPlan([], desired);

        Assert.Equal(2, plan.Count(x => x.Kind == DnsChangeKind.Create && x.Type == DnsRecordType.Mx));
    }

    [Fact]
    public void Planner_handles_ttl_changes()
    {
        IReadOnlyList<DnsRecordSnapshot> current =
        [
            new DnsRecordSnapshot("1", "app", DnsRecordType.A, "1.2.3.4", 3600, true),
        ];
        IReadOnlyList<DnsRecordSnapshot> desired =
        [
            new DnsRecordSnapshot("1", "app", DnsRecordType.A, "1.2.3.4", 7200, true),
        ];

        var plan = DnsPlanner.BuildPlan(current, desired);

        var change = Assert.Single(plan);
        Assert.Equal(DnsChangeKind.Update, change.Kind);
    }

    [Fact]
    public void Planner_handles_value_normalization()
    {
        IReadOnlyList<DnsRecordSnapshot> current =
        [
            new DnsRecordSnapshot("1", "app", DnsRecordType.A, "1.2.3.4.", 3600, true),
        ];
        IReadOnlyList<DnsRecordSnapshot> desired =
        [
            new DnsRecordSnapshot("1", "app", DnsRecordType.A, "1.2.3.4", 3600, true),
        ];

        var plan = DnsPlanner.BuildPlan(current, desired);

        var change = Assert.Single(plan);
        Assert.Equal(DnsChangeKind.NoOp, change.Kind);
        Assert.Contains("No change", change.RiskReason);
    }

    [Fact]
    public void SafetyGuard_converts_delete_of_protected_type_to_noop()
    {
        var change = new DnsPlanChange(
            DnsChangeKind.Delete,
            "@",
            DnsRecordType.Ns,
            "ns1.example.com",
            null,
            3600,
            "delete");

        var guarded = DnsSafetyRules.GuardChange(change);

        Assert.NotNull(guarded);
        Assert.Equal(DnsChangeKind.NoOp, guarded!.Kind);
        Assert.Contains("NS/SOA", guarded.RiskReason);
    }

    [Fact]
    public void SafetyGuard_converts_update_of_protected_type_to_noop()
    {
        var change = new DnsPlanChange(
            DnsChangeKind.Update,
            "@",
            DnsRecordType.Soa,
            "old",
            "new",
            3600,
            "update");

        var guarded = DnsSafetyRules.GuardChange(change);

        Assert.NotNull(guarded);
        Assert.Equal(DnsChangeKind.NoOp, guarded!.Kind);
        Assert.Contains("NS/SOA", guarded.RiskReason);
    }

    [Fact]
    public void SafetyGuard_passes_through_non_protected_types()
    {
        var change = new DnsPlanChange(
            DnsChangeKind.Delete,
            "app",
            DnsRecordType.A,
            "1.2.3.4",
            null,
            3600,
            "delete");

        var guarded = DnsSafetyRules.GuardChange(change);

        Assert.Null(guarded);
    }

    [Theory]
    [InlineData(DnsRecordType.A)]
    [InlineData(DnsRecordType.Aaaa)]
    [InlineData(DnsRecordType.Cname)]
    public void CanDelete_allows_managed_non_protected_types(DnsRecordType type)
    {
        var record = new DnsRecordSnapshot("1", "app", type, "value", 3600, true);

        Assert.True(DnsSafetyRules.CanDelete(record));
    }

    [Theory]
    [InlineData(DnsRecordType.Ns)]
    [InlineData(DnsRecordType.Soa)]
    public void CanDelete_blocks_protected_types(DnsRecordType type)
    {
        var record = new DnsRecordSnapshot("1", "@", type, "value", 3600, true);

        Assert.False(DnsSafetyRules.CanDelete(record));
    }

    [Fact]
    public void CanDelete_blocks_unowned_records()
    {
        var record = new DnsRecordSnapshot("1", "app", DnsRecordType.A, "value", 3600, false);

        Assert.False(DnsSafetyRules.CanDelete(record));
    }
}
