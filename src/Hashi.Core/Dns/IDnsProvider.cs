using Hashi.Core.Dns;

namespace Hashi.Core.Dns;

public interface IDnsProvider
{
    string ProviderType { get; }

    Task<IReadOnlyList<DnsZone>> ListZonesAsync(CancellationToken cancellationToken = default);

    Task<IReadOnlyList<DnsRecordSnapshot>> ListRecordsAsync(
        string zoneId,
        CancellationToken cancellationToken = default);

    Task<DnsRecordSnapshot> CreateRecordAsync(
        string zoneId,
        string name,
        DnsRecordType type,
        string value,
        int? ttl,
        CancellationToken cancellationToken = default);

    Task<DnsRecordSnapshot> UpdateRecordAsync(
        string recordId,
        string value,
        int? ttl,
        CancellationToken cancellationToken = default);

    Task DeleteRecordAsync(string recordId, CancellationToken cancellationToken = default);

    Task<DnsProviderCapabilities> GetCapabilitiesAsync(CancellationToken cancellationToken = default);
}

public interface IDnsProviderFactory
{
    IDnsProvider Create(string providerType, string apiToken);
}
