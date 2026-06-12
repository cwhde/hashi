using System.Net.Http.Headers;
using System.Net.Http.Json;
using System.Text.Json.Serialization;
using Hashi.Core.Dns;
using Hashi.Infrastructure.Persistence.Entities;
using Microsoft.Extensions.Logging;

namespace Hashi.Infrastructure.Providers.Dns;

public sealed class HetznerDnsProvider(HttpClient httpClient, string apiToken, ILogger<HetznerDnsProvider> logger)
    : IDnsProvider
{
    public string ProviderType => DnsProviderTypeNames.Hetzner;

    public async Task<IReadOnlyList<DnsZone>> ListZonesAsync(CancellationToken cancellationToken = default)
    {
        var response = await SendAsync(HttpMethod.Get, "zones", cancellationToken);
        var payload = await response.Content.ReadFromJsonAsync<HetznerZonesResponse>(cancellationToken)
            ?? throw new InvalidOperationException("Invalid Hetzner zones response.");
        return payload.Zones.Select(x => new DnsZone(x.Id, x.Name, x.Ttl ?? 3600)).ToList();
    }

    public async Task<IReadOnlyList<DnsRecordSnapshot>> ListRecordsAsync(
        string zoneId,
        CancellationToken cancellationToken = default)
    {
        var response = await SendAsync(HttpMethod.Get, $"zones/{zoneId}/records", cancellationToken);
        var payload = await response.Content.ReadFromJsonAsync<HetznerRecordsResponse>(cancellationToken)
            ?? throw new InvalidOperationException("Invalid Hetzner records response.");

        return payload.Records.Select(x => new DnsRecordSnapshot(
            x.Id,
            x.Name,
            DnsRecordTypeMapping.Parse(x.Type),
            x.Value,
            x.Ttl,
            IsManagedByHashi: false)).ToList();
    }

    public async Task<DnsRecordSnapshot> CreateRecordAsync(
        string zoneId,
        string name,
        DnsRecordType type,
        string value,
        int? ttl,
        CancellationToken cancellationToken = default)
    {
        var body = new
        {
            name,
            type = DnsRecordTypeMapping.ToApiName(type),
            value,
            ttl = ttl ?? 3600,
            zone_id = zoneId,
        };
        var response = await SendAsync(HttpMethod.Post, "records", cancellationToken, body);
        var payload = await response.Content.ReadFromJsonAsync<HetznerRecordResponse>(cancellationToken)
            ?? throw new InvalidOperationException("Invalid Hetzner create response.");
        var record = payload.Record;
        return new DnsRecordSnapshot(record.Id, record.Name, type, record.Value, record.Ttl, true);
    }

    public async Task<DnsRecordSnapshot> UpdateRecordAsync(
        string recordId,
        string value,
        int? ttl,
        CancellationToken cancellationToken = default)
    {
        var body = new { value, ttl };
        var response = await SendAsync(HttpMethod.Put, $"records/{recordId}", cancellationToken, body);
        var payload = await response.Content.ReadFromJsonAsync<HetznerRecordResponse>(cancellationToken)
            ?? throw new InvalidOperationException("Invalid Hetzner update response.");
        var record = payload.Record;
        return new DnsRecordSnapshot(
            record.Id,
            record.Name,
            DnsRecordTypeMapping.Parse(record.Type),
            record.Value,
            record.Ttl,
            true);
    }

    public async Task DeleteRecordAsync(string recordId, CancellationToken cancellationToken = default)
    {
        await SendAsync(HttpMethod.Delete, $"records/{recordId}", cancellationToken);
    }

    public Task<DnsProviderCapabilities> GetCapabilitiesAsync(CancellationToken cancellationToken = default)
        => Task.FromResult(new DnsProviderCapabilities(
            SupportedRecordTypes: ["A", "AAAA", "CNAME", "MX", "TXT"],
            SupportsBatchOperations: true,
            MaxRecordsPerZone: null,
            SupportsComments: false,
            RateLimitLimit: 60,
            RateLimitWindowSeconds: 60));

    public static async Task<(bool Valid, string? Error)> ValidateTokenAsync(
        HttpClient httpClient,
        string apiToken,
        CancellationToken cancellationToken = default)
    {
        using var request = new HttpRequestMessage(HttpMethod.Get, "zones");
        request.Headers.Add("Auth-API-Token", apiToken);
        var response = await httpClient.SendAsync(request, cancellationToken);
        return response.IsSuccessStatusCode
            ? (true, null)
            : (false, $"HTTP {(int)response.StatusCode}: {response.ReasonPhrase}");
    }

    private async Task<HttpResponseMessage> SendAsync(
        HttpMethod method,
        string path,
        CancellationToken cancellationToken,
        object? body = null)
    {
        using var request = new HttpRequestMessage(method, path);
        request.Headers.Add("Auth-API-Token", apiToken);
        if (body is not null)
        {
            request.Content = JsonContent.Create(body);
            request.Content.Headers.ContentType = new MediaTypeHeaderValue("application/json");
        }

        var response = await httpClient.SendAsync(request, cancellationToken);
        if (!response.IsSuccessStatusCode)
        {
            var error = await response.Content.ReadAsStringAsync(cancellationToken);
            logger.LogWarning("Hetzner DNS API {Method} {Path} failed: {Status} {Body}", method, path, response.StatusCode, error);
            response.EnsureSuccessStatusCode();
        }

        return response;
    }

    private sealed record HetznerZonesResponse(
        [property: JsonPropertyName("zones")] IReadOnlyList<HetznerZoneDto> Zones);

    private sealed record HetznerZoneDto(
        [property: JsonPropertyName("id")] string Id,
        [property: JsonPropertyName("name")] string Name,
        [property: JsonPropertyName("ttl")] int? Ttl);

    private sealed record HetznerRecordsResponse(
        [property: JsonPropertyName("records")] IReadOnlyList<HetznerRecordDto> Records);

    private sealed record HetznerRecordResponse(
        [property: JsonPropertyName("record")] HetznerRecordDto Record);

    private sealed record HetznerRecordDto(
        [property: JsonPropertyName("id")] string Id,
        [property: JsonPropertyName("name")] string Name,
        [property: JsonPropertyName("type")] string Type,
        [property: JsonPropertyName("value")] string Value,
        [property: JsonPropertyName("ttl")] int? Ttl);
}

public sealed class DnsProviderFactory(IHttpClientFactory httpClientFactory, ILogger<HetznerDnsProvider> logger)
    : IDnsProviderFactory
{
    public IDnsProvider Create(string providerType, string apiToken)
    {
        if (!string.Equals(providerType, Persistence.Entities.DnsProviderTypeNames.Hetzner, StringComparison.OrdinalIgnoreCase))
        {
            throw new NotSupportedException($"Unsupported DNS provider type: {providerType}");
        }

        var client = httpClientFactory.CreateClient("hetzner-dns");
        return new HetznerDnsProvider(client, apiToken, logger);
    }
}
