using System.Net;
using System.Text.Json;
using Hashi.Contracts.Api;
using Hashi.Core.Resources;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Infrastructure.Platform;

public sealed class EdgeAuthService(HashiDbContext db, GeoIpLookupService geoIp, OidcEdgeAuthService oidc)
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
        var clientIpText = clientIp.ToString();
        var manualBlocklistMatch = await FindMatchingBlocklistEntryAsync(
            clientIpText,
            countryCode,
            regionCode,
            asn,
            BlocklistSourceNames.Manual,
            cancellationToken);
        if (manualBlocklistMatch is not null)
        {
            manualBlocklistMatch.LastHitAtUtc = DateTimeOffset.UtcNow;
            await db.SaveChangesAsync(cancellationToken);
            return new EdgeAuthForwardResponse("deny", null);
        }

        var manuallyAllowed = await IsManuallyAllowedAsync(clientIp, countryCode, regionCode, asn, cancellationToken);
        if (!manuallyAllowed)
        {
            var blocklistMatch = await FindMatchingBlocklistEntryAsync(
                clientIpText,
                countryCode,
                regionCode,
                asn,
                null,
                cancellationToken);
            if (blocklistMatch is not null)
            {
                blocklistMatch.LastHitAtUtc = DateTimeOffset.UtcNow;
                await db.SaveChangesAsync(cancellationToken);
                return new EdgeAuthForwardResponse("deny", null);
            }
        }

        var normalizedHost = NormalizeForwardedHost(host);
        var rootDomain = (await db.AppSettings.AsNoTracking().SingleOrDefaultAsync(cancellationToken))?.RootDomain;
        var resources = await db.Resources.AsNoTracking()
            .Where(x => x.Enabled)
            .ToListAsync(cancellationToken);
        var resource = resources.FirstOrDefault(x => string.Equals(
            ResourceDomainResolver.Resolve(x.DomainMode, x.Domain, x.Slug, rootDomain),
            normalizedHost,
            StringComparison.OrdinalIgnoreCase));
        var hasOidcProvider = await db.OidcProviders.AsNoTracking().AnyAsync(x => x.Enabled, cancellationToken);
        var hasValidSession = await oidc.ValidateSessionAsync(edgeSessionKey, cancellationToken);

        if (resource is not null)
        {
            var resourceRuleResult = await EvaluateResourceRulesAsync(
                resource,
                normalizedHost,
                path,
                clientIp,
                countryCode,
                regionCode,
                asn,
                hasValidSession,
                hasOidcProvider,
                cancellationToken);
            if (resourceRuleResult is not null)
            {
                return resourceRuleResult;
            }
        }

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

        if (hasValidSession)
        {
            return new EdgeAuthForwardResponse("allow", null);
        }

        if (!hasOidcProvider && policy == ForwardAuthPolicy.SsoRequired)
        {
            return new EdgeAuthForwardResponse("deny", null);
        }

        if (policy == ForwardAuthPolicy.SsoRequired)
        {
            return new EdgeAuthForwardResponse("challenge", BuildLoginUrl(host, path));
        }

        var bucket = await db.AbuseBuckets.AsNoTracking()
            .SingleOrDefaultAsync(x => x.ClientIp == clientIpText, cancellationToken);
        if (manuallyAllowed)
        {
            return new EdgeAuthForwardResponse("allow", null);
        }

        var bucketState = SecuritySubjectStateNames.Normalize(bucket?.State);
        if (bucketState is SecuritySubjectStateNames.FirewallBlocked
            or SecuritySubjectStateNames.SoftBlocked
            or SecuritySubjectStateNames.ManuallyBlocked)
        {
            return new EdgeAuthForwardResponse("deny", null);
        }

        if (bucketState is SecuritySubjectStateNames.Suspect or SecuritySubjectStateNames.Challenged)
        {
            if (!hasOidcProvider)
            {
                return new EdgeAuthForwardResponse("deny", null);
            }

            return new EdgeAuthForwardResponse("challenge", BuildLoginUrl(host, path));
        }

        return new EdgeAuthForwardResponse("allow", null);
    }

    private async Task<EdgeAuthForwardResponse?> EvaluateResourceRulesAsync(
        Persistence.Entities.ResourceEntity resource,
        string host,
        string path,
        IPAddress clientIp,
        string? countryCode,
        string? regionCode,
        string? asn,
        bool hasValidSession,
        bool hasOidcProvider,
        CancellationToken cancellationToken)
    {
        var rules = await db.ResourceRules.AsNoTracking()
            .Where(x => x.ResourceId == resource.Id && x.Enabled)
            .OrderByDescending(x => x.Priority)
            .ToListAsync(cancellationToken);

        foreach (var rule in rules)
        {
            if (!MatchesResourceRule(rule, path, clientIp, countryCode, regionCode, asn))
            {
                continue;
            }

            if (!ResourceRuleActionNames.TryNormalize(rule.Action, out var action))
            {
                return null;
            }

            return action switch
            {
                ResourceRuleActionNames.BypassAuth => new EdgeAuthForwardResponse("allow", null),
                ResourceRuleActionNames.BlockAccess => new EdgeAuthForwardResponse("deny", null),
                ResourceRuleActionNames.RequireAdaptiveChallenge => AuthRuleDecision(host, path, hasValidSession, hasOidcProvider),
                ResourceRuleActionNames.PassToAuth => AuthRuleDecision(host, path, hasValidSession, hasOidcProvider),
                _ => null,
            };
        }

        return null;
    }

    private static EdgeAuthForwardResponse AuthRuleDecision(
        string host,
        string path,
        bool hasValidSession,
        bool hasOidcProvider)
    {
        if (hasValidSession)
        {
            return new EdgeAuthForwardResponse("allow", null);
        }

        return hasOidcProvider
            ? new EdgeAuthForwardResponse("challenge", BuildLoginUrl(host, path))
            : new EdgeAuthForwardResponse("deny", null);
    }

    public IReadOnlyList<string> ValidateRuleMatchJson(string matchJson)
        => geoIp.ValidateGeoMatchRules(matchJson);

    private static string BuildLoginUrl(string host, string path)
    {
        var returnUrl = Uri.EscapeDataString($"https://{host}{path}");
        return $"/api/edge-auth/login?returnUrl={returnUrl}";
    }

    private static string NormalizeForwardedHost(string host)
    {
        var normalized = host.Trim().ToLowerInvariant();
        var colonIndex = normalized.IndexOf(':', StringComparison.Ordinal);
        return colonIndex > 0
            ? normalized[..colonIndex]
            : normalized;
    }

    private async Task<BlocklistEntryEntity?> FindMatchingBlocklistEntryAsync(
        string clientIp,
        string? countryCode,
        string? regionCode,
        string? asn,
        string? source,
        CancellationToken cancellationToken)
    {
        var now = DateTimeOffset.UtcNow;
        var query = db.BlocklistEntries
            .Where(x => x.ExpiresAtUtc == null || x.ExpiresAtUtc > now)
            .AsQueryable();
        if (source is not null)
        {
            query = query.Where(x => x.Source == source);
        }
        else
        {
            query = query.Where(x => x.Source != BlocklistSourceNames.Manual);
        }

        var entries = await query.ToListAsync(cancellationToken);

        foreach (var entry in entries)
        {
            var type = NormalizeBlockType(entry);
            var value = NormalizeBlockValue(entry);
            if (type switch
            {
                Persistence.Entities.BlocklistTypeNames.Ip => string.Equals(clientIp, value, StringComparison.OrdinalIgnoreCase),
                Persistence.Entities.BlocklistTypeNames.Asn => string.Equals(asn, value, StringComparison.OrdinalIgnoreCase),
                Persistence.Entities.BlocklistTypeNames.Country => string.Equals(countryCode, value, StringComparison.OrdinalIgnoreCase),
                Persistence.Entities.BlocklistTypeNames.Region => string.Equals(regionCode, value, StringComparison.OrdinalIgnoreCase),
                _ => false,
            })
            {
                return entry;
            }
        }

        return null;
    }

    private async Task<bool> IsManuallyAllowedAsync(
        IPAddress clientIp,
        string? countryCode,
        string? regionCode,
        string? asn,
        CancellationToken cancellationToken)
    {
        var subjects = await db.FirewallAllowedSubjects.AsNoTracking()
            .Where(x => x.Enabled)
            .ToListAsync(cancellationToken);
        return subjects.Any(x => MatchesAllowedSubject(x, clientIp, countryCode, regionCode, asn));
    }

    private static bool MatchesAllowedSubject(
        FirewallAllowedSubjectEntity subject,
        IPAddress clientIp,
        string? countryCode,
        string? regionCode,
        string? asn)
        => subject.SubjectKind.Trim().ToLowerInvariant() switch
        {
            FirewallSubjectKindNames.Ip => string.Equals(subject.SubjectValue, clientIp.ToString(), StringComparison.OrdinalIgnoreCase),
            FirewallSubjectKindNames.Cidr => IsInCidr(clientIp, subject.SubjectValue),
            FirewallSubjectKindNames.Country => string.Equals(subject.SubjectValue, countryCode, StringComparison.OrdinalIgnoreCase),
            FirewallSubjectKindNames.Asn => string.Equals(subject.SubjectValue, asn, StringComparison.OrdinalIgnoreCase),
            _ => false,
        };

    private static string NormalizeBlockType(BlocklistEntryEntity entry)
        => string.IsNullOrWhiteSpace(entry.Type)
            ? BlocklistTypeNames.Ip
            : entry.Type.Trim().ToLowerInvariant();

    private static string NormalizeBlockValue(BlocklistEntryEntity entry)
    {
        if (!string.IsNullOrWhiteSpace(entry.Value))
        {
            return entry.Value.Trim();
        }

        return entry.ClientIp.Trim();
    }

    private static bool MatchesResourceRule(
        Persistence.Entities.ResourceRuleEntity rule,
        string path,
        IPAddress clientIp,
        string? countryCode,
        string? regionCode,
        string? asn)
        => rule.MatchType.ToLowerInvariant() switch
        {
            ResourceRuleMatchTypeNames.Ip => string.Equals(clientIp.ToString(), rule.MatchValue, StringComparison.OrdinalIgnoreCase),
            ResourceRuleMatchTypeNames.Cidr => IsInCidr(clientIp, rule.MatchValue),
            ResourceRuleMatchTypeNames.Path => path.StartsWith(rule.MatchValue, StringComparison.OrdinalIgnoreCase),
            ResourceRuleMatchTypeNames.Country => string.Equals(countryCode, rule.MatchValue, StringComparison.OrdinalIgnoreCase),
            ResourceRuleMatchTypeNames.Region => string.Equals(regionCode, rule.MatchValue, StringComparison.OrdinalIgnoreCase),
            ResourceRuleMatchTypeNames.Asn => string.Equals(asn, rule.MatchValue, StringComparison.OrdinalIgnoreCase),
            _ => false,
        };

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
