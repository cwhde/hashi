using System.Net;
using System.Text.Json;
using Hashi.Contracts.Api;
using Hashi.Core.Resources;
using Hashi.Infrastructure.Persistence;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Infrastructure.Platform;

public sealed class EdgeAuthService(HashiDbContext db, GeoIpLookupService geoIp)
{
    public async Task<EdgeAuthForwardResponse> EvaluateForwardAsync(
        string host,
        string path,
        IPAddress clientIp,
        string? countryCode,
        string? regionCode,
        string? asn,
        string? edgeSessionKey = null,
        string? mode = null,
        CancellationToken cancellationToken = default)
    {
        var rules = await db.EdgeAuthRules.AsNoTracking()
            .Where(x => x.Enabled)
            .OrderBy(x => x.Priority)
            .ToListAsync(cancellationToken);

        foreach (var rule in rules)
        {
            if (!Matches(rule.MatchJson, host, path, clientIp, countryCode, regionCode, asn))
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

        if (string.Equals(mode, "observe", StringComparison.OrdinalIgnoreCase))
        {
            return new EdgeAuthForwardResponse("allow", null);
        }

        var clientIpText = clientIp.ToString();
        if (await db.BlocklistEntries.AsNoTracking().AnyAsync(x => x.ClientIp == clientIpText, cancellationToken))
        {
            return new EdgeAuthForwardResponse("deny", null);
        }

        var resource = await db.Resources.AsNoTracking()
            .Where(x => x.Enabled && x.Domain != null)
            .FirstOrDefaultAsync(x => string.Equals(x.Domain, host, StringComparison.OrdinalIgnoreCase), cancellationToken);
        var policy = resource is null
            ? ForwardAuthPolicy.Adaptive
            : ForwardAuthPolicyMapping.Parse(resource.ForwardAuthPolicy);

        if (string.Equals(mode, "strict", StringComparison.OrdinalIgnoreCase))
        {
            policy = ForwardAuthPolicy.SsoRequired;
        }

        if (policy == ForwardAuthPolicy.Off || policy == ForwardAuthPolicy.Observe)
        {
            return new EdgeAuthForwardResponse("allow", null);
        }

        var hasSession = HasValidEdgeSession(edgeSessionKey);
        if (hasSession)
        {
            return new EdgeAuthForwardResponse("allow", null);
        }

        var providers = await db.OidcProviders.AsNoTracking().AnyAsync(x => x.Enabled, cancellationToken);
        if (!providers)
        {
            return new EdgeAuthForwardResponse("allow", null);
        }

        if (policy == ForwardAuthPolicy.SsoRequired)
        {
            return new EdgeAuthForwardResponse("challenge", BuildLoginUrl(host, path));
        }

        var bucket = await db.AbuseBuckets.AsNoTracking()
            .SingleOrDefaultAsync(x => x.ClientIp == clientIpText, cancellationToken);
        if (bucket?.State is "challenge" or "block")
        {
            return bucket.State == "block"
                ? new EdgeAuthForwardResponse("deny", null)
                : new EdgeAuthForwardResponse("challenge", BuildLoginUrl(host, path));
        }

        return new EdgeAuthForwardResponse("allow", null);
    }

    private static string BuildLoginUrl(string host, string path)
    {
        var returnUrl = Uri.EscapeDataString($"https://{host}{path}");
        return $"/api/edge-auth/login?returnUrl={returnUrl}";
    }

    public IReadOnlyList<string> ValidateRuleMatchJson(string matchJson)
        => geoIp.ValidateGeoMatchRules(matchJson);

    private static bool HasValidEdgeSession(string? edgeSessionKey)
        => OidcEdgeAuthService.TryValidateSession(edgeSessionKey);

    private static bool Matches(
        string matchJson,
        string host,
        string path,
        IPAddress clientIp,
        string? countryCode,
        string? regionCode,
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

        if (root.TryGetProperty("region", out var regionMatch)
            && !string.Equals(regionCode, regionMatch.GetString(), StringComparison.OrdinalIgnoreCase))
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
