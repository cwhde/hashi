using System.Net;
using System.Net.Sockets;
using Hashi.Infrastructure.Persistence.Entities;

namespace Hashi.Infrastructure.Platform;

public sealed record NormalizedSecuritySubject(string SubjectType, string SubjectValue, string NormalizedValue);

public static class SecuritySubjectNormalizer
{
    public static bool TryNormalize(string? subjectType, string? subjectValue, out NormalizedSecuritySubject subject)
    {
        subject = new NormalizedSecuritySubject(string.Empty, subjectValue?.Trim() ?? string.Empty, string.Empty);
        if (string.IsNullOrWhiteSpace(subjectType) || string.IsNullOrWhiteSpace(subjectValue))
        {
            return false;
        }

        var type = subjectType.Trim().ToLowerInvariant();
        var value = subjectValue.Trim();
        return type switch
        {
            SecuritySubjectTypeNames.Ip => TryNormalizeIp(value, out subject),
            SecuritySubjectTypeNames.Cidr => TryNormalizeCidr(value, out subject),
            SecuritySubjectTypeNames.Asn => TryNormalizeAsn(value, out subject),
            SecuritySubjectTypeNames.Country => TryNormalizeCountry(value, out subject),
            SecuritySubjectTypeNames.Region => TryNormalizeRegion(value, out subject),
            SecuritySubjectTypeNames.Session => TryNormalizeOpaque(SecuritySubjectTypeNames.Session, value, out subject),
            SecuritySubjectTypeNames.Composite => TryNormalizeOpaque(SecuritySubjectTypeNames.Composite, value, out subject),
            _ => false,
        };
    }

    public static NormalizedSecuritySubject Normalize(string subjectType, string subjectValue)
        => TryNormalize(subjectType, subjectValue, out var subject)
            ? subject
            : throw new InvalidOperationException($"Invalid security subject '{subjectType}:{subjectValue}'.");

    public static NormalizedSecuritySubject NormalizeIp(IPAddress ip)
    {
        var normalized = NormalizeIpAddress(ip);
        return new NormalizedSecuritySubject(SecuritySubjectTypeNames.Ip, normalized, normalized);
    }

    public static bool Matches(
        string subjectType,
        string normalizedValue,
        IPAddress clientIp,
        string? countryCode,
        string? regionCode,
        string? asn)
    {
        var clientSubject = NormalizeIp(clientIp);
        return subjectType.Trim().ToLowerInvariant() switch
        {
            SecuritySubjectTypeNames.Ip => string.Equals(normalizedValue, clientSubject.NormalizedValue, StringComparison.OrdinalIgnoreCase),
            SecuritySubjectTypeNames.Cidr => IsInCidr(clientIp, normalizedValue),
            SecuritySubjectTypeNames.Country => TryNormalizeCountry(countryCode, out var country)
                && string.Equals(normalizedValue, country.NormalizedValue, StringComparison.OrdinalIgnoreCase),
            SecuritySubjectTypeNames.Region => TryNormalizeRegion(regionCode, out var region)
                && string.Equals(normalizedValue, region.NormalizedValue, StringComparison.OrdinalIgnoreCase),
            SecuritySubjectTypeNames.Asn => TryNormalizeAsn(asn, out var normalizedAsn)
                && string.Equals(normalizedValue, normalizedAsn.NormalizedValue, StringComparison.OrdinalIgnoreCase),
            _ => false,
        };
    }

    public static bool IsInCidr(IPAddress ip, string cidr)
    {
        if (!TryNormalizeCidr(cidr, out var normalizedCidr))
        {
            return false;
        }

        var parts = normalizedCidr.NormalizedValue.Split('/', 2);
        var network = IPAddress.Parse(parts[0]);
        var prefixLength = int.Parse(parts[1], System.Globalization.CultureInfo.InvariantCulture);
        var ipBytes = NormalizeAddressFamily(ip, network.AddressFamily).GetAddressBytes();
        var networkBytes = network.GetAddressBytes();
        if (ipBytes.Length != networkBytes.Length)
        {
            return false;
        }

        var fullBytes = prefixLength / 8;
        var remainingBits = prefixLength % 8;
        for (var i = 0; i < fullBytes; i++)
        {
            if (ipBytes[i] != networkBytes[i])
            {
                return false;
            }
        }

        if (remainingBits == 0)
        {
            return true;
        }

        var mask = (byte)(0xFF << (8 - remainingBits));
        return (ipBytes[fullBytes] & mask) == (networkBytes[fullBytes] & mask);
    }

    private static bool TryNormalizeIp(string? value, out NormalizedSecuritySubject subject)
    {
        subject = new NormalizedSecuritySubject(SecuritySubjectTypeNames.Ip, value?.Trim() ?? string.Empty, string.Empty);
        if (!IPAddress.TryParse(value, out var ip))
        {
            return false;
        }

        var normalized = NormalizeIpAddress(ip);
        subject = new NormalizedSecuritySubject(SecuritySubjectTypeNames.Ip, normalized, normalized);
        return true;
    }

    private static bool TryNormalizeCidr(string? value, out NormalizedSecuritySubject subject)
    {
        subject = new NormalizedSecuritySubject(SecuritySubjectTypeNames.Cidr, value?.Trim() ?? string.Empty, string.Empty);
        if (string.IsNullOrWhiteSpace(value))
        {
            return false;
        }

        var parts = value.Trim().Split('/', 2, StringSplitOptions.TrimEntries);
        if (parts.Length != 2
            || !IPAddress.TryParse(parts[0], out var address)
            || !int.TryParse(parts[1], out var prefixLength))
        {
            return false;
        }

        address = NormalizeMappedAddress(address);
        var byteCount = address.AddressFamily == AddressFamily.InterNetwork ? 4 : 16;
        if (prefixLength < 0 || prefixLength > byteCount * 8)
        {
            return false;
        }

        var bytes = address.GetAddressBytes();
        var fullBytes = prefixLength / 8;
        var remainingBits = prefixLength % 8;
        if (remainingBits > 0 && fullBytes < bytes.Length)
        {
            bytes[fullBytes] &= (byte)(0xFF << (8 - remainingBits));
            fullBytes++;
        }

        for (var i = fullBytes; i < bytes.Length; i++)
        {
            bytes[i] = 0;
        }

        var network = new IPAddress(bytes);
        var normalized = $"{NormalizeIpAddress(network)}/{prefixLength}";
        subject = new NormalizedSecuritySubject(SecuritySubjectTypeNames.Cidr, normalized, normalized);
        return true;
    }

    private static bool TryNormalizeAsn(string? value, out NormalizedSecuritySubject subject)
    {
        subject = new NormalizedSecuritySubject(SecuritySubjectTypeNames.Asn, value?.Trim() ?? string.Empty, string.Empty);
        if (string.IsNullOrWhiteSpace(value))
        {
            return false;
        }

        var candidate = value.Trim();
        if (candidate.StartsWith("AS", StringComparison.OrdinalIgnoreCase))
        {
            candidate = candidate[2..];
        }

        if (!long.TryParse(candidate, out var number) || number <= 0)
        {
            return false;
        }

        var normalized = $"AS{number}";
        subject = new NormalizedSecuritySubject(SecuritySubjectTypeNames.Asn, normalized, normalized);
        return true;
    }

    private static bool TryNormalizeCountry(string? value, out NormalizedSecuritySubject subject)
    {
        subject = new NormalizedSecuritySubject(SecuritySubjectTypeNames.Country, value?.Trim() ?? string.Empty, string.Empty);
        if (string.IsNullOrWhiteSpace(value))
        {
            return false;
        }

        var normalized = value.Trim().ToUpperInvariant();
        if (normalized.Length != 2 || normalized.Any(ch => ch < 'A' || ch > 'Z'))
        {
            return false;
        }

        subject = new NormalizedSecuritySubject(SecuritySubjectTypeNames.Country, normalized, normalized);
        return true;
    }

    private static bool TryNormalizeRegion(string? value, out NormalizedSecuritySubject subject)
    {
        subject = new NormalizedSecuritySubject(SecuritySubjectTypeNames.Region, value?.Trim() ?? string.Empty, string.Empty);
        if (string.IsNullOrWhiteSpace(value))
        {
            return false;
        }

        var normalized = value.Trim().ToUpperInvariant();
        subject = new NormalizedSecuritySubject(SecuritySubjectTypeNames.Region, normalized, normalized);
        return true;
    }

    private static bool TryNormalizeOpaque(string subjectType, string value, out NormalizedSecuritySubject subject)
    {
        var normalized = value.Trim().ToLowerInvariant();
        subject = new NormalizedSecuritySubject(subjectType, value.Trim(), normalized);
        return normalized.Length > 0;
    }

    private static string NormalizeIpAddress(IPAddress ip)
    {
        var normalized = NormalizeMappedAddress(ip);
        return normalized.ToString().ToLowerInvariant();
    }

    private static IPAddress NormalizeMappedAddress(IPAddress ip)
        => ip.IsIPv4MappedToIPv6 ? ip.MapToIPv4() : ip;

    private static IPAddress NormalizeAddressFamily(IPAddress ip, AddressFamily addressFamily)
    {
        ip = NormalizeMappedAddress(ip);
        if (ip.AddressFamily == addressFamily)
        {
            return ip;
        }

        return addressFamily == AddressFamily.InterNetworkV6 && ip.AddressFamily == AddressFamily.InterNetwork
            ? ip.MapToIPv6()
            : ip;
    }
}
