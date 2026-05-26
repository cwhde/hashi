using System.Net;
using System.Text.Json;
using MaxMind.GeoIP2;
using MaxMind.GeoIP2.Exceptions;
using Microsoft.Extensions.Configuration;
using Microsoft.Extensions.Logging;

namespace Hashi.Infrastructure.Platform;

public sealed record GeoIpLookup(string? CountryCode, string? RegionCode, string? Asn);

public sealed class GeoIpLookupService(IConfiguration configuration, ILogger<GeoIpLookupService> logger)
{
    private readonly object _sync = new();
    private readonly string _dataPath = configuration["Hashi:DataPath"] is { Length: > 0 } path
        ? Path.Combine(path, "geoip")
        : "/data/geoip";
    private DatabaseReader? _cityReader;
    private DatabaseReader? _asnReader;
    private DateTimeOffset _lastLoadAttemptUtc = DateTimeOffset.MinValue;

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
                var city = _cityReader.City(address);
                country = city.Country.IsoCode;
                region = city.MostSpecificSubdivision.IsoCode;
            }
            catch (AddressNotFoundException)
            {
                // Unknown address is valid; leave geo fields empty.
            }
            catch (Exception ex)
            {
                logger.LogDebug(ex, "GeoIP city lookup failed for {Address}", address);
            }
        }

        if (_asnReader is not null)
        {
            try
            {
                var asnResponse = _asnReader.Asn(address);
                asn = asnResponse.AutonomousSystemNumber is long number ? $"AS{number}" : null;
            }
            catch (AddressNotFoundException)
            {
            }
            catch (Exception ex)
            {
                logger.LogDebug(ex, "GeoIP ASN lookup failed for {Address}", address);
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
                logger.LogInformation("GeoIP databases not found in {DataPath}", _dataPath);
            }
        }
    }

    private DatabaseReader? TryOpenReader(string fileName)
    {
        var path = Path.Combine(_dataPath, fileName);
        if (!File.Exists(path))
        {
            return null;
        }

        try
        {
            return new DatabaseReader(path);
        }
        catch (Exception ex)
        {
            logger.LogWarning(ex, "Failed to open GeoIP database {Path}", path);
            return null;
        }
    }
}
