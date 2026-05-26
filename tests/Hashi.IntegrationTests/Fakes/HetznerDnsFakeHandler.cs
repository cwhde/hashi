using System.Collections.Concurrent;
using System.Net;
using System.Text.Json;

namespace Hashi.IntegrationTests.Fakes;

public sealed class HetznerDnsFakeHandler : HttpMessageHandler
{
    private readonly ConcurrentDictionary<string, FakeZone> _zones = new(StringComparer.OrdinalIgnoreCase);
    private readonly ConcurrentDictionary<string, FakeRecord> _records = new(StringComparer.OrdinalIgnoreCase);
    private int _recordCounter;

    public HetznerDnsFakeHandler()
    {
        var zoneId = "zone-test";
        _zones[zoneId] = new FakeZone(zoneId, "example.com", 3600);
        AddRecord(zoneId, "ns1", "NS", "ns1.example.com.", 3600);
        AddRecord(zoneId, "app", "A", "1.2.3.4", 3600);
    }

    public string DefaultZoneId => "zone-test";

    protected override Task<HttpResponseMessage> SendAsync(HttpRequestMessage request, CancellationToken cancellationToken)
    {
        if (!request.Headers.TryGetValues("Auth-API-Token", out _))
        {
            return Task.FromResult(Json(HttpStatusCode.Unauthorized, new { error = "missing token" }));
        }

        var path = request.RequestUri?.AbsolutePath.Trim('/') ?? string.Empty;
        if (path.StartsWith("api/v1/", StringComparison.OrdinalIgnoreCase))
        {
            path = path["api/v1/".Length..];
        }
        if (path is "zones")
        {
            return Task.FromResult(Json(HttpStatusCode.OK, new
            {
                zones = _zones.Values.Select(z => new { id = z.Id, name = z.Name, ttl = z.Ttl }).ToArray(),
            }));
        }

        if (path.StartsWith("zones/", StringComparison.Ordinal) && path.EndsWith("/records", StringComparison.Ordinal))
        {
            var zoneId = path.Split('/')[1];
            var records = _records.Values.Where(x => x.ZoneId == zoneId)
                .Select(x => new { id = x.Id, name = x.Name, type = x.Type, value = x.Value, ttl = x.Ttl })
                .ToArray();
            return Task.FromResult(Json(HttpStatusCode.OK, new { records }));
        }

        if (path == "records" && request.Method == HttpMethod.Post)
        {
            return CreateFromBodyAsync(request, cancellationToken);
        }

        if (path.StartsWith("records/", StringComparison.Ordinal))
        {
            var recordId = path.Split('/')[1];
            if (request.Method == HttpMethod.Delete)
            {
                _records.TryRemove(recordId, out _);
                return Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK));
            }

            if (request.Method == HttpMethod.Put && _records.TryGetValue(recordId, out var existing))
            {
                return UpdateFromBodyAsync(request, existing, cancellationToken);
            }
        }

        return Task.FromResult(new HttpResponseMessage(HttpStatusCode.NotFound));
    }

    private async Task<HttpResponseMessage> CreateFromBodyAsync(HttpRequestMessage request, CancellationToken cancellationToken)
    {
        using var doc = await JsonDocument.ParseAsync(await request.Content!.ReadAsStreamAsync(cancellationToken), cancellationToken: cancellationToken);
        var root = doc.RootElement;
        var zoneId = root.GetProperty("zone_id").GetString() ?? DefaultZoneId;
        var name = root.GetProperty("name").GetString() ?? string.Empty;
        var type = root.GetProperty("type").GetString() ?? "A";
        var value = root.GetProperty("value").GetString() ?? string.Empty;
        var ttl = root.TryGetProperty("ttl", out var ttlElement) ? ttlElement.GetInt32() : 3600;
        var record = AddRecord(zoneId, name, type, value, ttl);
        return Json(HttpStatusCode.OK, new { record = new { id = record.Id, name = record.Name, type = record.Type, value = record.Value, ttl = record.Ttl } });
    }

    private async Task<HttpResponseMessage> UpdateFromBodyAsync(HttpRequestMessage request, FakeRecord existing, CancellationToken cancellationToken)
    {
        using var doc = await JsonDocument.ParseAsync(await request.Content!.ReadAsStreamAsync(cancellationToken), cancellationToken: cancellationToken);
        var root = doc.RootElement;
        if (root.TryGetProperty("value", out var valueElement))
        {
            existing.Value = valueElement.GetString() ?? existing.Value;
        }

        if (root.TryGetProperty("ttl", out var ttlElement))
        {
            existing.Ttl = ttlElement.GetInt32();
        }

        return Json(HttpStatusCode.OK, new { record = new { id = existing.Id, name = existing.Name, type = existing.Type, value = existing.Value, ttl = existing.Ttl } });
    }

    private FakeRecord AddRecord(string zoneId, string name, string type, string value, int ttl)
    {
        var id = Interlocked.Increment(ref _recordCounter).ToString();
        var record = new FakeRecord(id, zoneId, name, type, value, ttl);
        _records[id] = record;
        return record;
    }

    private static HttpResponseMessage Json(HttpStatusCode status, object payload)
        => new(status) { Content = new StringContent(JsonSerializer.Serialize(payload), System.Text.Encoding.UTF8, "application/json") };

    private sealed record FakeZone(string Id, string Name, int Ttl);

    private sealed class FakeRecord(string id, string zoneId, string name, string type, string value, int ttl)
    {
        public string Id { get; } = id;
        public string ZoneId { get; } = zoneId;
        public string Name { get; } = name;
        public string Type { get; } = type;
        public string Value { get; set; } = value;
        public int Ttl { get; set; } = ttl;
    }
}
