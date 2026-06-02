using System.Net;
using System.Text.Json;
using MaxMind.GeoIP2;
using MaxMind.GeoIP2.Exceptions;
using Microsoft.Extensions.Configuration;
using Microsoft.Extensions.Logging;

namespace Hashi.Infrastructure.Platform;

public sealed record GeoIpLookup(string? CountryCode, string? RegionCode, string? Asn);

public sealed class GeoIpLookupService : IDisposable
{
    private readonly object _sync = new();
    private readonly string _dataPath;
    private readonly ILogger<GeoIpLookupService> _logger;
    private readonly IGeoIpDatabaseReaderFactory _readerFactory;
    private IGeoIpDatabaseReader? _cityReader;
    private IGeoIpDatabaseReader? _asnReader;
    private DateTimeOffset _lastLoadAttemptUtc = DateTimeOffset.MinValue;

    public GeoIpLookupService(IConfiguration configuration, ILogger<GeoIpLookupService> logger)
        : this(configuration, logger, new MaxMindGeoIpDatabaseReaderFactory())
    {
    }

    internal GeoIpLookupService(
        IConfiguration configuration,
        ILogger<GeoIpLookupService> logger,
        IGeoIpDatabaseReaderFactory readerFactory)
    {
        _dataPath = configuration["Hashi:DataPath"] is { Length: > 0 } path
            ? Path.Combine(path, "geoip")
            : "/data/geoip";
        _logger = logger;
        _readerFactory = readerFactory;
    }

    public bool IsAvailable
    {
        get
        {
            EnsureReadersLoaded();
            return _cityReader is not null || _asnReader is not null;
        }
    }

    public GeoIpLookup? Lookup(IPAddress address)
    {
        EnsureReadersLoaded();
        string? country = null;
        string? region = null;
        string? asn = null;

        if (_cityReader is not null)
        {
            try
            {
                var city = _cityReader.LookupCity(address);
                country = city.CountryCode;
                region = city.RegionCode;
            }
            catch (AddressNotFoundException)
            {
                // Unknown address is valid; leave geo fields empty.
            }
            catch (Exception ex)
            {
                _logger.LogDebug(ex, "GeoIP city lookup failed for {Address}", address);
            }
        }

        if (_asnReader is not null)
        {
            try
            {
                asn = _asnReader.LookupAsn(address);
            }
            catch (AddressNotFoundException)
            {
            }
            catch (Exception ex)
            {
                _logger.LogDebug(ex, "GeoIP ASN lookup failed for {Address}", address);
            }
        }

        if (country is null && region is null && asn is null)
        {
            return null;
        }

        return new GeoIpLookup(country, region, asn);
    }

    public IReadOnlyList<string> ValidateGeoMatchRules(string matchJson)
    {
        using var doc = JsonDocument.Parse(matchJson);
        var root = doc.RootElement;
        var requiresGeo = root.TryGetProperty("country", out _)
            || root.TryGetProperty("region", out _)
            || root.TryGetProperty("asn", out _);
        if (!requiresGeo || IsAvailable)
        {
            return [];
        }

        return ["Country, region, and ASN rules require a GeoIP database under /data/geoip."];
    }

    public void Reload()
    {
        lock (_sync)
        {
            DisposeReaders();
            _lastLoadAttemptUtc = DateTimeOffset.MinValue;
        }
    }

    public void Dispose()
    {
        lock (_sync)
        {
            DisposeReaders();
        }
    }

    private void EnsureReadersLoaded()
    {
        if (_cityReader is not null || _asnReader is not null)
        {
            return;
        }

        if (DateTimeOffset.UtcNow - _lastLoadAttemptUtc < TimeSpan.FromMinutes(1))
        {
            return;
        }

        lock (_sync)
        {
            if (_cityReader is not null || _asnReader is not null)
            {
                return;
            }

            _lastLoadAttemptUtc = DateTimeOffset.UtcNow;
            _cityReader = TryOpenReader("GeoLite2-City.mmdb") ?? TryOpenReader("GeoLite2-Country.mmdb");
            _asnReader = TryOpenReader("GeoLite2-ASN.mmdb");
            if (_cityReader is null && _asnReader is null)
            {
                _logger.LogInformation("GeoIP databases not found in {DataPath}", _dataPath);
            }
        }
    }

    private IGeoIpDatabaseReader? TryOpenReader(string fileName)
    {
        var path = Path.Combine(_dataPath, fileName);
        if (!File.Exists(path))
        {
            return null;
        }

        try
        {
            return _readerFactory.Open(path);
        }
        catch (Exception ex)
        {
            _logger.LogWarning(ex, "Failed to open GeoIP database {Path}", path);
            return null;
        }
    }

    private void DisposeReaders()
    {
        _cityReader?.Dispose();
        _asnReader?.Dispose();
        _cityReader = null;
        _asnReader = null;
    }
}

internal interface IGeoIpDatabaseReader : IDisposable
{
    GeoIpLookup LookupCity(IPAddress address);

    string? LookupAsn(IPAddress address);
}

internal interface IGeoIpDatabaseReaderFactory
{
    IGeoIpDatabaseReader Open(string path);
}

internal sealed class MaxMindGeoIpDatabaseReaderFactory : IGeoIpDatabaseReaderFactory
{
    public IGeoIpDatabaseReader Open(string path) => new MaxMindGeoIpDatabaseReader(path);
}

internal sealed class MaxMindGeoIpDatabaseReader : IGeoIpDatabaseReader
{
    private readonly DatabaseReader _reader;

    public MaxMindGeoIpDatabaseReader(string path)
    {
        _reader = new DatabaseReader(path);
    }

    public GeoIpLookup LookupCity(IPAddress address)
    {
        var city = _reader.City(address);
        return new GeoIpLookup(city.Country.IsoCode, city.MostSpecificSubdivision.IsoCode, null);
    }

    public string? LookupAsn(IPAddress address)
    {
        var asnResponse = _reader.Asn(address);
        return asnResponse.AutonomousSystemNumber is long number ? $"AS{number}" : null;
    }

    public void Dispose() => _reader.Dispose();
}
