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
}
