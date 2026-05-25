using Hashi.Core.Dns;

namespace Hashi.Core.Dns;

public static class DnsPlanner
{
    public static IReadOnlyList<DnsPlanChange> BuildPlan(
        IReadOnlyList<DnsRecordSnapshot> current,
        IReadOnlyList<DnsRecordSnapshot> desired)
    {
        var changes = new List<DnsPlanChange>();
        var currentByKey = current.ToDictionary(Key, StringComparer.OrdinalIgnoreCase);
        var desiredByKey = desired.ToDictionary(Key, StringComparer.OrdinalIgnoreCase);

        foreach (var (key, desiredRecord) in desiredByKey)
        {
            if (!currentByKey.TryGetValue(key, out var existing))
            {
                changes.Add(Guard(new DnsPlanChange(
                    DnsChangeKind.Create,
                    desiredRecord.Name,
                    desiredRecord.Type,
                    null,
                    desiredRecord.Value,
                    desiredRecord.Ttl,
                    "Create managed record.")));
                continue;
            }

            if (!string.Equals(Normalize(existing.Value), Normalize(desiredRecord.Value), StringComparison.OrdinalIgnoreCase)
                || existing.Ttl != desiredRecord.Ttl)
            {
                changes.Add(Guard(new DnsPlanChange(
                    DnsChangeKind.Update,
                    desiredRecord.Name,
                    desiredRecord.Type,
                    existing.Value,
                    desiredRecord.Value,
                    desiredRecord.Ttl,
                    "Update managed record.")));
            }
            else
            {
                changes.Add(Guard(new DnsPlanChange(
                    DnsChangeKind.NoOp,
                    desiredRecord.Name,
                    desiredRecord.Type,
                    existing.Value,
                    desiredRecord.Value,
                    desiredRecord.Ttl,
                    "No change.")));
            }
        }

        foreach (var (key, existing) in currentByKey)
        {
            if (desiredByKey.ContainsKey(key) || !existing.IsManagedByHashi)
            {
                continue;
            }

            changes.Add(Guard(new DnsPlanChange(
                DnsChangeKind.Delete,
                existing.Name,
                existing.Type,
                existing.Value,
                null,
                existing.Ttl,
                "Remove stale managed record.")));
        }

        return changes;
    }

    private static DnsPlanChange Guard(DnsPlanChange change)
        => DnsSafetyRules.GuardChange(change) ?? change;

    private static string Key(DnsRecordSnapshot record)
        => $"{record.Name}|{DnsRecordTypeMapping.ToApiName(record.Type)}";

    private static string Normalize(string value) => value.Trim().TrimEnd('.');
}
