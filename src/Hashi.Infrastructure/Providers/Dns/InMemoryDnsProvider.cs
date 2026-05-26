using Hashi.Core.Dns;
using Hashi.Infrastructure.Persistence.Entities;

namespace Hashi.Infrastructure.Providers.Dns;

public sealed class InMemoryDnsProvider : IDnsProvider
{
    private readonly Dictionary<string, DnsZone> _zones = new(StringComparer.OrdinalIgnoreCase);
    private readonly Dictionary<string, List<DnsRecordSnapshot>> _records = new(StringComparer.OrdinalIgnoreCase);
    private int _recordCounter;

    public string ProviderType => DnsProviderTypeNames.Hetzner;

    public void SeedZone(string zoneId, string name, params DnsRecordSnapshot[] records)
    {
        _zones[zoneId] = new DnsZone(zoneId, name, 3600);
        _records[zoneId] = records.ToList();
    }

    public Task<IReadOnlyList<DnsZone>> ListZonesAsync(CancellationToken cancellationToken = default)
        => Task.FromResult<IReadOnlyList<DnsZone>>(_zones.Values.ToList());

    public Task<IReadOnlyList<DnsRecordSnapshot>> ListRecordsAsync(string zoneId, CancellationToken cancellationToken = default)
        => Task.FromResult<IReadOnlyList<DnsRecordSnapshot>>(_records.TryGetValue(zoneId, out var records) ? records : []);

    public Task<DnsRecordSnapshot> CreateRecordAsync(
        string zoneId,
        string name,
        DnsRecordType type,
        string value,
        int? ttl,
        CancellationToken cancellationToken = default)
    {
        var record = new DnsRecordSnapshot($"rec-{++_recordCounter}", name, type, value, ttl, true);
        if (!_records.TryGetValue(zoneId, out var list))
        {
            list = [];
            _records[zoneId] = list;
        }

        list.Add(record);
        return Task.FromResult(record);
    }

    public Task<DnsRecordSnapshot> UpdateRecordAsync(
        string recordId,
        string value,
        int? ttl,
        CancellationToken cancellationToken = default)
    {
        foreach (var list in _records.Values)
        {
            var index = list.FindIndex(x => x.ProviderRecordId == recordId);
            if (index >= 0)
            {
                var existing = list[index];
                var updated = existing with { Value = value, Ttl = ttl };
                list[index] = updated;
                return Task.FromResult(updated);
            }
        }

        throw new KeyNotFoundException($"Record {recordId} not found.");
    }

    public Task DeleteRecordAsync(string recordId, CancellationToken cancellationToken = default)
    {
        foreach (var list in _records.Values)
        {
            var removed = list.RemoveAll(x => x.ProviderRecordId == recordId);
            if (removed > 0)
            {
                return Task.CompletedTask;
            }
        }

        throw new KeyNotFoundException($"Record {recordId} not found.");
    }
}
