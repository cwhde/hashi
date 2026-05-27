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

    private static string Key(DnsRecordSnapshot record)
        => $"{record.Name}|{DnsRecordTypeMapping.ToApiName(record.Type)}";

    private static string Normalize(string value) => value.Trim().TrimEnd('.');
}
