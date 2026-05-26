using System.Net;
using System.Text.Json;
using Hashi.Contracts.Api;
using Hashi.Infrastructure.Persistence;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Infrastructure.Platform;

public sealed class EdgeAuthService(HashiDbContext db)
{
    public async Task<EdgeAuthForwardResponse> EvaluateForwardAsync(
        string host,
        string path,
        IPAddress clientIp,
        string? countryCode,
        string? asn,
        string? edgeSessionKey = null,
        CancellationToken cancellationToken = default)
    {
        var rules = await db.EdgeAuthRules.AsNoTracking()
            .Where(x => x.Enabled)
            .OrderBy(x => x.Priority)
            .ToListAsync(cancellationToken);

        foreach (var rule in rules)
        {
            if (!Matches(rule.MatchJson, host, path, clientIp, countryCode, asn))
            {
                continue;
            }

            return rule.Action switch
            {
                "deny" => new EdgeAuthForwardResponse("deny", null),
                "redirect" => new EdgeAuthForwardResponse("redirect", "/auth/login"),
                _ => new EdgeAuthForwardResponse("allow", null),
            };
        }

        var providers = await db.OidcProviders.AsNoTracking().AnyAsync(x => x.Enabled, cancellationToken);
        if (providers && HasValidEdgeSession(edgeSessionKey))
        {
            return new EdgeAuthForwardResponse("allow", null);
        }

        return providers
            ? new EdgeAuthForwardResponse("challenge", "/api/edge-auth/login")
            : new EdgeAuthForwardResponse("allow", null);
    }

    private static bool HasValidEdgeSession(string? edgeSessionKey)
        => !string.IsNullOrWhiteSpace(edgeSessionKey)
           && EdgeSessionStore.TryGet(edgeSessionKey, out var session)
           && session is not null
           && session.ExpiresAtUtc > DateTimeOffset.UtcNow;

    private static bool Matches(
        string matchJson,
        string host,
        string path,
        IPAddress clientIp,
        string? countryCode,
        string? asn)
    {
        using var doc = JsonDocument.Parse(matchJson);
        var root = doc.RootElement;
        if (root.TryGetProperty("host", out var hostMatch)
            && !host.Contains(hostMatch.GetString() ?? string.Empty, StringComparison.OrdinalIgnoreCase))
        {
            return false;
        }

        if (root.TryGetProperty("pathPrefix", out var pathMatch)
            && !path.StartsWith(pathMatch.GetString() ?? string.Empty, StringComparison.OrdinalIgnoreCase))
        {
            return false;
        }

        if (root.TryGetProperty("cidr", out var cidrMatch)
            && !IsInCidr(clientIp, cidrMatch.GetString() ?? string.Empty))
        {
            return false;
        }

        if (root.TryGetProperty("country", out var countryMatch)
            && !string.Equals(countryCode, countryMatch.GetString(), StringComparison.OrdinalIgnoreCase))
        {
            return false;
        }

        if (root.TryGetProperty("asn", out var asnMatch)
            && !string.Equals(asn, asnMatch.GetString(), StringComparison.OrdinalIgnoreCase))
        {
            return false;
        }

        return true;
    }

    private static bool IsInCidr(IPAddress ip, string cidr)
    {
        if (string.IsNullOrWhiteSpace(cidr) || !cidr.Contains('/'))
        {
            return false;
        }

        var parts = cidr.Split('/');
        if (!IPAddress.TryParse(parts[0], out var network))
        {
            return false;
        }

        if (!int.TryParse(parts[1], out var prefixLength))
        {
            return false;
        }

        var ipBytes = ip.GetAddressBytes();
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
}
