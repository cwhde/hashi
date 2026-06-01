using Hashi.Core.Dns;

namespace Hashi.Core.Dns;

public static class DnsPlanner
{
    public static IReadOnlyList<DnsPlanChange> BuildPlan(
        IReadOnlyList<DnsRecordSnapshot> current,
        IReadOnlyList<DnsRecordSnapshot> desired)
    {
        var changes = new List<DnsPlanChange>();
        var currentByKey = BuildIndex(current);
        var desiredGroups = desired
            .GroupBy(Key, StringComparer.OrdinalIgnoreCase)
            .ToList();
        var desiredByKey = desiredGroups.ToDictionary(x => x.Key, x => x.First(), StringComparer.OrdinalIgnoreCase);

        foreach (var group in desiredGroups.Where(x => x.Count() > 1))
        {
            var records = group.ToList();
            var first = records[0];
            foreach (var duplicate in records.Skip(1))
            {
                changes.Add(Guard(new DnsPlanChange(
                    DnsChangeKind.NoOp,
                    duplicate.Name,
                    duplicate.Type,
                    first.Value,
                    duplicate.Value,
                    duplicate.Ttl,
                    "Desired DNS records conflict on the same name and type; resolve the manual/imported record before Hashi can create the generated record.")));
            }
        }

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
                if (!existing.IsManagedByHashi)
                {
                    changes.Add(Guard(new DnsPlanChange(
                        DnsChangeKind.NoOp,
                        desiredRecord.Name,
                        desiredRecord.Type,
                        existing.Value,
                        desiredRecord.Value,
                        desiredRecord.Ttl,
                        "Provider record is not owned by Hashi; import or assign ownership before updating.")
                    {
                        ProviderRecordId = existing.ProviderRecordId,
                    }));
                    continue;
                }

                changes.Add(Guard(new DnsPlanChange(
                    DnsChangeKind.Update,
                    desiredRecord.Name,
                    desiredRecord.Type,
                    existing.Value,
                    desiredRecord.Value,
                    desiredRecord.Ttl,
                    "Update managed record.")
                {
                    ProviderRecordId = existing.ProviderRecordId,
                }));
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
                    existing.IsManagedByHashi
                        ? "No change."
                        : "Provider record is not owned by Hashi; import or assign ownership before Hashi can manage it.")
                {
                    ProviderRecordId = existing.ProviderRecordId,
                }));
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
                "Remove stale managed record.")
            {
                ProviderRecordId = existing.ProviderRecordId,
            }));
        }

        return changes;
    }

    private static DnsPlanChange Guard(DnsPlanChange change)
        => DnsSafetyRules.GuardChange(change) ?? change;

    private static Dictionary<string, DnsRecordSnapshot> BuildIndex(IReadOnlyList<DnsRecordSnapshot> records)
    {
        var index = new Dictionary<string, DnsRecordSnapshot>(StringComparer.OrdinalIgnoreCase);
        foreach (var record in records
            .OrderBy(x => x.ProviderRecordId, StringComparer.OrdinalIgnoreCase)
            .ThenBy(x => x.Value, StringComparer.OrdinalIgnoreCase))
        {
            index.TryAdd(Key(record), record);
        }

        return index;
    }

    private static string Key(DnsRecordSnapshot record)
    {
        var key = $"{record.Name}|{DnsRecordTypeMapping.ToApiName(record.Type)}";
        return IsMultiValue(record.Type)
            ? $"{key}|{Normalize(record.Value)}"
            : key;
    }

    private static bool IsMultiValue(DnsRecordType type)
        => type is DnsRecordType.Mx or DnsRecordType.Txt;

    private static string Normalize(string value) => value.Trim().TrimEnd('.');
}
